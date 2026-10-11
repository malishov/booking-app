package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/muradalishov/booking-app/backend/migrations"
)

func TestBusinessAliasesAndStableReceipt(t *testing.T) {
	ctx, tx, q, b, s := snapshotTestDB(t, false)
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Minute)
	if err := q.InsertBooking(ctx, b, s, "Client", "+994500000333", "stable-receipt", start); err != nil {
		t.Fatal(err)
	}
	addresses := []string{b.Slug, "renamed-studio", "third-studio", b.Slug}
	for i, address := range addresses[1:] {
		current, err := q.SaveBusiness(ctx, SaveBusinessParams{OwnerUserID: b.OwnerUserID, Name: b.Name, Slug: address, Timezone: b.Timezone})
		if err != nil {
			t.Fatal(err)
		}
		for _, old := range addresses[:i+2] {
			resolved, err := q.GetBusinessBySlug(ctx, old)
			if err != nil || resolved.ID != b.ID || resolved.Slug != current.Slug {
				t.Fatalf("alias lost: %s, %v", old, err)
			}
		}
		profile, err := q.GetReceiptBusiness(ctx, pgtype.Text{String: "stable-receipt", Valid: true})
		if err != nil || profile.ID != b.ID || profile.Slug != address {
			t.Fatal("receipt depends on old slug", err)
		}
	}
	other, err := q.CreateUser(ctx, CreateUserParams{Email: "alias-other@example.test", PasswordHash: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	// The address is no longer the current businesses.slug, but remains reserved.
	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(nested).SaveBusiness(ctx, SaveBusinessParams{OwnerUserID: other.ID, Name: "Other", Slug: "renamed-studio", Timezone: "UTC"})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "businesses_slug_key" {
		t.Fatal("old alias hijacked", err)
	}
	if err := nested.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetBusinessByOwner(ctx, other.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("failed claim left a business", err)
	}
	if _, err := q.SaveBusiness(ctx, SaveBusinessParams{OwnerUserID: other.ID, Name: "Other", Slug: "other-studio", Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	nested, err = tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(nested).SaveBusiness(ctx, SaveBusinessParams{OwnerUserID: other.ID, Name: "Other", Slug: "third-studio", Timezone: "UTC"})
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatal("existing business hijacked an alias", err)
	}
	if err := nested.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	untouched, err := q.GetBusinessByOwner(ctx, other.ID)
	if err != nil || untouched.Slug != "other-studio" {
		t.Fatal("failed rename changed profile", err)
	}
	if _, err := q.GetReceiptBusiness(ctx, pgtype.Text{String: "wrong-token-hash", Valid: true}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("invalid receipt hash accepted", err)
	}
}

func TestBusinessAliasesUpgrade(t *testing.T) {
	ctx, tx, q, b, _ := snapshotTestDB(t, false)
	raw, err := migrations.FS.ReadFile("00008_business_slug_aliases.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), "-- +goose Down", 2)
	if _, err := tx.Exec(ctx, parts[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, parts[0]); err != nil {
		t.Fatal(err)
	}
	resolved, err := q.GetBusinessBySlug(ctx, b.Slug)
	if err != nil || resolved.ID != b.ID {
		t.Fatal("existing business not backfilled", err)
	}
	if _, err := q.SaveBusiness(ctx, SaveBusinessParams{OwnerUserID: b.OwnerUserID, Name: b.Name, Slug: "after-upgrade", Timezone: b.Timezone}); err != nil {
		t.Fatal(err)
	}
	resolved, err = q.GetBusinessBySlug(ctx, b.Slug)
	if err != nil || resolved.Slug != "after-upgrade" {
		t.Fatal("upgrade did not retain old address", err)
	}
}
