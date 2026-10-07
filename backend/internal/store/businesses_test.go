package store

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/muradalishov/booking-app/backend/migrations"
)

// TestBusinessPersistence runs migrations and queries in an isolated transaction.
// Set TEST_DATABASE_URL to enable; rollback removes the temporary schema and data.
func TestBusinessPersistence(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	schema := pgx.Identifier{"booking_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+schema+"; SET LOCAL search_path TO "+schema+", public"); err != nil {
		t.Fatal(err)
	}
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		sql, err := migrations.FS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.SplitN(string(sql), "-- +goose Down", 2)[0]
		if _, err := tx.Exec(ctx, up); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	queries := New(tx)
	owner, err := queries.CreateUser(ctx, CreateUserParams{Email: "owner@example.test", PasswordHash: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := queries.CreateUser(ctx, CreateUserParams{Email: "other@example.test", PasswordHash: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	params := SaveBusinessParams{OwnerUserID: owner.ID, Name: "Анна", Slug: "anna-nails", Description: "Маникюр", Timezone: "Asia/Dubai"}
	created, err := queries.SaveBusiness(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	params.Name = "Студия Анны"
	params.Slug = "anna-studio"
	updated, err := queries.SaveBusiness(ctx, params)
	if err != nil || updated.ID != created.ID {
		t.Fatalf("edit created a second business: %v", err)
	}
	byOwner, err := queries.GetBusinessByOwner(ctx, owner.ID)
	if err != nil || byOwner.Name != params.Name || byOwner.Description != params.Description {
		t.Fatal("profile was not persisted")
	}
	_, err = queries.GetBusinessBySlug(ctx, "anna-nails")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("old slug still resolves")
	}
	public, err := queries.GetBusinessBySlug(ctx, params.Slug)
	if err != nil || public.ID != created.ID {
		t.Fatal("new public slug does not resolve")
	}
	_, err = queries.GetBusinessByOwner(ctx, other.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("other owner can read profile")
	}
	// Use a savepoint because a unique violation aborts its transaction scope.
	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	params.OwnerUserID = other.ID
	_, err = New(nested).SaveBusiness(ctx, params)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "businesses_slug_key" {
		t.Fatalf("duplicate slug accepted: %v", err)
	}
	if err := nested.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	service, err := queries.CreateService(ctx, CreateServiceParams{OwnerUserID: owner.ID, Name: "Маникюр", DurationMinutes: 90, PriceCents: 3510, Currency: "AZN"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = queries.GetServiceByOwner(ctx, GetServiceByOwnerParams{ID: service.ID, OwnerUserID: other.ID})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("other owner can read service")
	}
	_, err = queries.UpdateService(ctx, UpdateServiceParams{ID: service.ID, OwnerUserID: other.ID, Name: "Foreign edit", DurationMinutes: 1, PriceCents: 1, Currency: "USD"})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("other owner can update service")
	}
	_, err = queries.ToggleService(ctx, ToggleServiceParams{ID: service.ID, OwnerUserID: other.ID})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("other owner can toggle service")
	}
	updatedService, err := queries.UpdateService(ctx, UpdateServiceParams{ID: service.ID, OwnerUserID: owner.ID, Name: "Маникюр с покрытием", DurationMinutes: 100, PriceCents: 4099, Currency: "AZN"})
	if err != nil || updatedService.PriceCents != 4099 {
		t.Fatal("service edit did not persist exact price")
	}
	publicServices, err := queries.ListPublicServices(ctx, created.ID)
	if err != nil || len(publicServices) != 1 {
		t.Fatal("active service not public")
	}
	if _, err := queries.ToggleService(ctx, ToggleServiceParams{ID: service.ID, OwnerUserID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	publicServices, err = queries.ListPublicServices(ctx, created.ID)
	if err != nil || len(publicServices) != 0 {
		t.Fatal("hidden service still public")
	}
	privateServices, err := queries.ListServicesByOwner(ctx, owner.ID)
	if err != nil || len(privateServices) != 1 || privateServices[0].IsActive {
		t.Fatal("hidden service missing from cabinet")
	}
	if _, err := queries.ToggleService(ctx, ToggleServiceParams{ID: service.ID, OwnerUserID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	publicServices, err = queries.ListPublicServices(ctx, created.ID)
	if err != nil || len(publicServices) != 1 {
		t.Fatal("restored service not public")
	}
}
