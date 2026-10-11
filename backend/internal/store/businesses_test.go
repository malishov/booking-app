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
	alias, err := queries.GetBusinessBySlug(ctx, "anna-nails")
	if err != nil || alias.ID != created.ID || alias.Slug != "anna-studio" {
		t.Fatal("old slug did not resolve to the current profile")
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
	// Weekly settings persist atomically and cannot be read or written by an owner
	// without a business. The surrounding transaction rolls all test data back.
	rawSchedule := []byte(`{"Days":[{"Index":0,"Enabled":true,"Start":"09:00","End":"18:00"}],"DaysOff":"2026-12-31"}`)
	if err := queries.SaveSchedule(ctx, owner.ID, rawSchedule); err != nil {
		t.Fatal(err)
	}
	storedSchedule, err := queries.GetSchedule(ctx, owner.ID)
	if err != nil || !strings.Contains(string(storedSchedule), "2026-12-31") {
		t.Fatal("schedule did not persist")
	}
	if err := queries.SaveSchedule(ctx, other.ID, rawSchedule); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("other owner wrote a schedule")
	}
	if _, err := queries.GetSchedule(ctx, other.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("other owner read a schedule")
	}
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Minute)
	if err := queries.InsertBooking(ctx, created, updatedService, "Анна", "+994501234567", "receipt-test", start); err != nil {
		t.Fatal(err)
	}
	receipt, err := queries.GetReceipt(ctx, created.ID, "receipt-test")
	if err != nil || receipt.PriceCents != 4099 || receipt.Duration != 100 || receipt.Name != "" || receipt.Phone != "" {
		t.Fatal("receipt snapshot or privacy failed")
	}
	conflictTx, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = New(conflictTx).InsertBooking(ctx, created, updatedService, "Другой клиент", "+994501234568", "conflict-test", start.Add(15*time.Minute))
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatalf("overlap accepted: %v", err)
	}
	if err := conflictTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := queries.InsertBooking(ctx, created, updatedService, "Следующий клиент", "+994501234569", "adjacent-test", start.Add(100*time.Minute)); err != nil {
		t.Fatalf("adjacent booking rejected: %v", err)
	}
	if _, err := queries.UpdateService(ctx, UpdateServiceParams{ID: service.ID, OwnerUserID: owner.ID, Name: "New service", DurationMinutes: 30, PriceCents: 9900, Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	receipt, err = queries.GetReceipt(ctx, created.ID, "receipt-test")
	if err != nil || receipt.PriceCents != 4099 || receipt.Duration != 100 || receipt.Currency != "AZN" {
		t.Fatal("historical snapshot changed")
	}
	owned, err := queries.ListOwnerBookings(ctx, owner.ID)
	if err != nil || len(owned) != 2 {
		t.Fatal("owner bookings missing")
	}
	foreign, err := queries.ListOwnerBookings(ctx, other.ID)
	if err != nil || len(foreign) != 0 {
		t.Fatal("private bookings leaked")
	}
	// Owner isolation, allowed transitions and availability after cancellation.
	dayRows, err := queries.ListOwnerDayBookings(ctx, owner.ID, start, start.Add(100*time.Minute))
	if err != nil || len(dayRows) != 1 || dayRows[0].ID != receipt.ID {
		t.Fatal("day interval boundaries incorrect")
	}
	foreignDay, err := queries.ListOwnerDayBookings(ctx, other.ID, start, start.Add(100*time.Minute))
	if err != nil || len(foreignDay) != 0 {
		t.Fatal("day calendar leaked to another owner")
	}
	now := start.Add(-time.Hour)
	// Rescheduling preserves the snapshot and receipt, releases the old window,
	// rejects foreign/stale/past requests and keeps the exclusion constraint.
	if _, err := queries.GetOwnerBooking(ctx, other.ID, receipt.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign owner read reschedule details")
	}
	if err := queries.RescheduleBooking(ctx, other.ID, receipt.ID, start, start.Add(24*time.Hour), now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign owner rescheduled visit")
	}
	if err := queries.RescheduleBooking(ctx, owner.ID, receipt.ID, start, start.Add(24*time.Hour), start); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("started visit rescheduled")
	}
	if err := queries.RescheduleBooking(ctx, owner.ID, receipt.ID, start, now, now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("visit moved to past")
	}
	withoutSelf, err := queries.ListBusyExcept(ctx, created.ID, start, start.Add(100*time.Minute), receipt.ID)
	if err != nil || len(withoutSelf) != 0 {
		t.Fatal("own visit blocks reschedule choices")
	}
	conflictTx, err = tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = New(conflictTx).RescheduleBooking(ctx, owner.ID, receipt.ID, start, start.Add(100*time.Minute), now)
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatalf("overlapping move accepted: %v", err)
	}
	if err := conflictTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	moved := start.Add(24 * time.Hour)
	if err := queries.RescheduleBooking(ctx, owner.ID, receipt.ID, start, moved, now); err != nil {
		t.Fatal(err)
	}
	current, err := queries.GetReceipt(ctx, created.ID, "receipt-test")
	if err != nil || !current.Start.Equal(moved) || !current.End.Equal(moved.Add(100*time.Minute)) || current.PriceCents != 4099 || current.Currency != "AZN" || current.ServiceName != "Маникюр с покрытием" {
		t.Fatal("reschedule changed snapshot or receipt")
	}
	oldBusy, err := queries.ListBusy(ctx, created.ID, start, start.Add(100*time.Minute))
	if err != nil || len(oldBusy) != 0 {
		t.Fatal("old slot still occupied after move")
	}
	if err := queries.RescheduleBooking(ctx, owner.ID, receipt.ID, start, moved.Add(time.Hour), now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("stale move overwrote newer visit")
	}
	if err := queries.RescheduleBooking(ctx, owner.ID, receipt.ID, moved, start, now); err != nil {
		t.Fatal(err)
	}
	if err := queries.ChangeBookingStatus(ctx, other.ID, receipt.ID, "cancelled", now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign owner changed booking")
	}
	for _, status := range []string{"completed", "no_show", "confirmed", "invalid"} {
		if err := queries.ChangeBookingStatus(ctx, owner.ID, receipt.ID, status, now); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("invalid future transition accepted: %s", status)
		}
	}
	if err := queries.ChangeBookingStatus(ctx, owner.ID, receipt.ID, "cancelled", now); err != nil {
		t.Fatal(err)
	}
	if err := queries.RescheduleBooking(ctx, owner.ID, receipt.ID, start, start.Add(24*time.Hour), now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cancelled visit rescheduled")
	}
	if err := queries.ChangeBookingStatus(ctx, owner.ID, receipt.ID, "completed", start.Add(200*time.Minute)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cancelled booking changed again")
	}
	busy, err := queries.ListBusy(ctx, created.ID, start, start.Add(100*time.Minute))
	if err != nil || len(busy) != 0 {
		t.Fatal("cancelled booking still occupies time")
	}
	if err := queries.InsertBooking(ctx, created, updatedService, "Новый клиент", "+994501234570", "replacement-test", start); err != nil {
		t.Fatalf("cancelled slot not reusable: %v", err)
	}
	replacement, err := queries.GetReceipt(ctx, created.ID, "replacement-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := queries.ChangeBookingStatus(ctx, owner.ID, replacement.ID, "completed", start.Add(100*time.Minute)); err != nil {
		t.Fatal(err)
	}
	busy, err = queries.ListBusy(ctx, created.ID, start, start.Add(100*time.Minute))
	if err != nil || len(busy) != 1 {
		t.Fatal("completed booking lost occupancy")
	}
	adjacent, err := queries.GetReceipt(ctx, created.ID, "adjacent-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := queries.ChangeBookingStatus(ctx, owner.ID, adjacent.ID, "no_show", adjacent.Start); err != nil {
		t.Fatal(err)
	}
	busy, err = queries.ListBusy(ctx, created.ID, adjacent.Start, adjacent.End)
	if err != nil || len(busy) != 0 {
		t.Fatal("no-show booking still occupies time")
	}
	localDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	history, err := queries.ListManagedBookings(ctx, owner.ID, "history", now, localDay, localDay.AddDate(0, 0, 1))
	if err != nil || len(history) != 3 {
		t.Fatal("terminal visits missing in history")
	}
	upcoming, err := queries.ListManagedBookings(ctx, owner.ID, "upcoming", now, localDay, localDay.AddDate(0, 0, 1))
	if err != nil || len(upcoming) != 0 {
		t.Fatal("terminal visits listed as upcoming")
	}
	foreign, err = queries.ListManagedBookings(ctx, other.ID, "history", now, localDay, localDay.AddDate(0, 0, 1))
	if err != nil || len(foreign) != 0 {
		t.Fatal("managed list leaked to another owner")
	}
}
