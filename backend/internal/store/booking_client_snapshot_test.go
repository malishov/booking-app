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

// Snapshot tests run in their own schema and transaction, without changing
// local/demo data. The pre-snapshot schema exercises an actual upgrade.
func snapshotTestDB(t *testing.T, legacy bool) (context.Context, pgx.Tx, *Queries, Business, Service) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback(context.Background()) })
	schema := pgx.Identifier{"snapshot_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)}.Sanitize()
	if _, err = tx.Exec(ctx, "CREATE SCHEMA "+schema+"; SET LOCAL search_path TO "+schema+", public"); err != nil {
		t.Fatal(err)
	}
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if legacy && file >= "00007_" {
			continue
		}
		raw, err := migrations.FS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, strings.SplitN(string(raw), "-- +goose Down", 2)[0]); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	q := New(tx)
	owner, err := q.CreateUser(ctx, CreateUserParams{Email: "snapshot@example.test", PasswordHash: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.SaveBusiness(ctx, SaveBusinessParams{OwnerUserID: owner.ID, Slug: "snapshot-studio", Name: "Snapshot studio", Timezone: "Asia/Baku"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := q.CreateService(ctx, CreateServiceParams{OwnerUserID: owner.ID, Name: "Manicure", DurationMinutes: 60, PriceCents: 3500, Currency: "AZN"})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, tx, q, b, s
}

func TestBookingContactsUpgrade(t *testing.T) {
	ctx, tx, _, b, _ := snapshotTestDB(t, true)
	var employee int64
	if err := tx.QueryRow(ctx, `INSERT INTO employees(business_id,user_id,name) VALUES($1,$2,'Master') RETURNING id`, b.ID, b.OwnerUserID).Scan(&employee); err != nil {
		t.Fatal(err)
	}
	for _, phone := range []any{"+994500000111", nil} {
		var customer, id int64
		if err := tx.QueryRow(ctx, `INSERT INTO customers(business_id,name,phone) VALUES($1,'Legacy client',$2) RETURNING id`, b.ID, phone).Scan(&customer); err != nil {
			t.Fatal(err)
		}
		start := time.Now().UTC().Add(time.Duration(customer) * 2 * time.Hour)
		if err := tx.QueryRow(ctx, `INSERT INTO bookings(business_id,employee_id,customer_id,starts_at,ends_at) VALUES($1,$2,$3,$4,$5) RETURNING id`, b.ID, employee, customer, start, start.Add(time.Hour)).Scan(&id); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := migrations.FS.ReadFile("00007_booking_client_snapshot.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, strings.SplitN(string(raw), "-- +goose Down", 2)[0]); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM bookings b JOIN customers c ON c.id=b.customer_id WHERE b.client_name=c.name AND b.client_phone=COALESCE(c.phone,'')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy contacts not backfilled: %d, %v", count, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO customers(business_id,name,phone) VALUES($1,'Shared phone client','+994500000111')`, b.ID); err != nil {
		t.Fatalf("shared phone still treated as identity: %v", err)
	}
}

func TestBookingContactsRemainIndependent(t *testing.T) {
	ctx, tx, q, b, s := snapshotTestDB(t, false)
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Minute)
	phone := "+994500000222"
	for i, name := range []string{"Original client", "Different person", "Original client"} {
		if err := q.InsertBooking(ctx, b, s, name, phone, "snapshot-"+strconv.Itoa(i), start.Add(time.Duration(i)*2*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	var count, identities int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*),COUNT(DISTINCT customer_id) FROM bookings WHERE business_id=$1`, b.ID).Scan(&count, &identities); err != nil || count != 3 || identities != 3 {
		t.Fatal("public input implicitly merged clients", err)
	}
	var firstName string
	if err := tx.QueryRow(ctx, `SELECT c.name FROM customers c JOIN bookings b ON b.customer_id=c.id WHERE b.receipt_hash='snapshot-0'`).Scan(&firstName); err != nil || firstName != "Original client" {
		t.Fatal("existing customer renamed", err)
	}
	// Future owner edits to the directory must not rewrite any visit contact.
	if _, err := tx.Exec(ctx, `UPDATE customers SET name='Directory edit',phone='+994500000999' WHERE business_id=$1`, b.ID); err != nil {
		t.Fatal(err)
	}
	first, err := q.GetReceipt(ctx, b.ID, "snapshot-0")
	if err != nil || first.Name != "" || first.Phone != "" {
		t.Fatal("receipt leaked client contacts", err)
	}
	check := func(rows []BookingView, err error) {
		t.Helper()
		if err != nil || len(rows) != 3 {
			t.Fatal("visit contacts missing", err)
		}
		for _, v := range rows {
			want := "Original client"
			if v.Start.Equal(start.Add(2 * time.Hour)) {
				want = "Different person"
			}
			if v.Name != want || v.Phone != phone {
				t.Fatalf("historical contacts changed: %+v", v)
			}
		}
	}
	check(q.ListOwnerBookings(ctx, b.OwnerUserID))
	check(q.ListOwnerDayBookings(ctx, b.OwnerUserID, start, start.Add(6*time.Hour)))
	check(q.ListManagedBookings(ctx, b.OwnerUserID, "upcoming", start.Add(-time.Hour), start, start.Add(24*time.Hour)))
	owned, err := q.GetOwnerBooking(ctx, b.OwnerUserID, first.ID)
	if err != nil || owned.Name != "Original client" || owned.Phone != phone {
		t.Fatal("reschedule details use directory", err)
	}
	if err := q.RescheduleBooking(ctx, b.OwnerUserID, first.ID, start, start.Add(24*time.Hour), start.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	owned, err = q.GetOwnerBooking(ctx, b.OwnerUserID, first.ID)
	if err != nil || owned.Name != "Original client" || owned.Phone != phone {
		t.Fatal("move changed client snapshot", err)
	}
	// A rejected overlapping booking must roll back its new customer too.
	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = New(nested).InsertBooking(ctx, b, s, "Failed booking", phone, "snapshot-conflict", start.Add(2*time.Hour))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatal("overlap not rejected", err)
	}
	if err := nested.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM customers WHERE business_id=$1`, b.ID).Scan(&count); err != nil || count != 3 {
		t.Fatal("failed booking left a customer", err)
	}
	if err := q.ChangeBookingStatus(ctx, b.OwnerUserID, first.ID, "cancelled", start.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	history, err := q.ListManagedBookings(ctx, b.OwnerUserID, "history", start.Add(-time.Hour), start, start.Add(24*time.Hour))
	if err != nil || len(history) != 1 || history[0].Name != "Original client" || history[0].Phone != phone {
		t.Fatal("history lost original contact", err)
	}
}
