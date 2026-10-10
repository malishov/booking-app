package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ChangeBookingStatus shares the business lock with booking creation. Only an
// active visit may change status; completing a future visit is forbidden.
func (q *Queries) ChangeBookingStatus(ctx context.Context, owner, id int64, status string, now time.Time) error {
	var business int64
	if err := q.db.QueryRow(ctx, `SELECT id FROM businesses WHERE owner_user_id=$1 FOR UPDATE`, owner).Scan(&business); err != nil {
		return err
	}
	result, err := q.db.Exec(ctx, `UPDATE bookings SET status=$3 WHERE id=$1 AND business_id=$2
 AND status='confirmed' AND ($3='cancelled' OR ($3='completed' AND ends_at<=$4) OR ($3='no_show' AND starts_at<=$4))`, id, business, status, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (q *Queries) ListManagedBookings(ctx context.Context, owner int64, filter string, now, today, tomorrow time.Time) ([]BookingView, error) {
	rows, err := q.db.Query(ctx, `SELECT b.id,b.starts_at,b.ends_at,b.client_name,b.client_phone,i.service_name,i.price_cents,i.duration_minutes,i.currency,b.status
 FROM bookings b JOIN businesses p ON p.id=b.business_id JOIN booking_items i ON i.booking_id=b.id
 WHERE p.owner_user_id=$1 AND (
 ($2='today' AND b.starts_at>=$4 AND b.starts_at<$5) OR
 ($2='upcoming' AND b.ends_at>$3 AND b.status='confirmed') OR
 ($2='history' AND (b.ends_at<=$3 OR b.status IN ('cancelled','completed','no_show'))))
 ORDER BY CASE WHEN $2!='history' THEN b.starts_at END ASC, b.starts_at DESC, b.id DESC LIMIT 200`, owner, filter, now, today, tomorrow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BookingView{}
	for rows.Next() {
		var v BookingView
		if err := rows.Scan(&v.ID, &v.Start, &v.End, &v.Name, &v.Phone, &v.ServiceName, &v.PriceCents, &v.Duration, &v.Currency, &v.Status); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

type BusyInterval struct{ Start, End time.Time }
type BookingView struct {
	ID                                         int64
	Start, End                                 time.Time
	Name, Phone, ServiceName, Currency, Status string
	PriceCents, Duration                       int32
}

func (q *Queries) ListOwnerDayBookings(ctx context.Context, owner int64, start, end time.Time) ([]BookingView, error) {
	rows, err := q.db.Query(ctx, `SELECT b.id,b.starts_at,b.ends_at,b.client_name,b.client_phone,i.service_name,i.price_cents,i.duration_minutes,i.currency,b.status
 FROM bookings b JOIN businesses p ON p.id=b.business_id JOIN booking_items i ON i.booking_id=b.id
 WHERE p.owner_user_id=$1 AND b.starts_at<$3 AND b.ends_at>$2 ORDER BY b.starts_at,b.id`, owner, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BookingView{}
	for rows.Next() {
		var v BookingView
		if err := rows.Scan(&v.ID, &v.Start, &v.End, &v.Name, &v.Phone, &v.ServiceName, &v.PriceCents, &v.Duration, &v.Currency, &v.Status); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (q *Queries) ListBusy(ctx context.Context, business int64, start, end time.Time) ([]BusyInterval, error) {
	return q.ListBusyExcept(ctx, business, start, end, 0)
}

// ListBusyExcept excludes only the visit being moved, so its own window can be
// reused while every other active booking remains occupied.
func (q *Queries) ListBusyExcept(ctx context.Context, business int64, start, end time.Time, except int64) ([]BusyInterval, error) {
	rows, err := q.db.Query(ctx, `SELECT starts_at,ends_at FROM bookings WHERE business_id=$1
 AND status NOT IN ('cancelled','no_show') AND starts_at<$3 AND ends_at>$2 AND id!=$4`, business, start, end, except)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BusyInterval{}
	for rows.Next() {
		var b BusyInterval
		if err := rows.Scan(&b.Start, &b.End); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

func (q *Queries) GetOwnerBooking(ctx context.Context, owner, id int64) (BookingView, error) {
	var v BookingView
	err := q.db.QueryRow(ctx, `SELECT b.id,b.starts_at,b.ends_at,b.client_name,b.client_phone,i.service_name,i.price_cents,i.duration_minutes,i.currency,b.status
 FROM bookings b JOIN businesses p ON p.id=b.business_id JOIN booking_items i ON i.booking_id=b.id
 WHERE p.owner_user_id=$1 AND b.id=$2`, owner, id).
		Scan(&v.ID, &v.Start, &v.End, &v.Name, &v.Phone, &v.ServiceName, &v.PriceCents, &v.Duration, &v.Currency, &v.Status)
	return v, err
}

// RescheduleBooking must run in a transaction. The shared business lock
// serializes moves with creation and status changes. The old start prevents a
// stale form from overwriting a newer move; item snapshots and receipt stay intact.
func (q *Queries) RescheduleBooking(ctx context.Context, owner, id int64, expected, start, now time.Time) error {
	var business int64
	if err := q.db.QueryRow(ctx, `SELECT id FROM businesses WHERE owner_user_id=$1 FOR UPDATE`, owner).Scan(&business); err != nil {
		return err
	}
	result, err := q.db.Exec(ctx, `UPDATE bookings b SET starts_at=$4,
 ends_at=$4+(SELECT SUM(duration_minutes) FROM booking_items WHERE booking_id=b.id)*INTERVAL '1 minute'
 WHERE b.id=$1 AND b.business_id=$2 AND b.starts_at=$3 AND b.status='confirmed' AND b.starts_at>$5 AND $4>$5`, id, business, expected, start, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
func (q *Queries) LockBookingBusiness(ctx context.Context, id int64) error {
	var value int64
	if err := q.db.QueryRow(ctx, `SELECT id FROM businesses WHERE id=$1 FOR UPDATE`, id).Scan(&value); err != nil {
		return err
	}
	if _, err := q.db.Exec(ctx, `SELECT id FROM services WHERE business_id=$1 AND is_active FOR SHARE`, id); err != nil {
		return err
	}
	_, err := q.db.Exec(ctx, `SELECT business_id FROM master_schedules WHERE business_id=$1 FOR SHARE`, id)
	return err
}

// InsertBooking runs inside the caller's transaction, so a conflict rolls back
// the customer, employee and snapshot together.
func (q *Queries) InsertBooking(ctx context.Context, b Business, s Service, name, phone, hash string, start time.Time) error {
	var employee, customer, booking int64
	err := q.db.QueryRow(ctx, `INSERT INTO employees(business_id,user_id,name) VALUES($1,$2,$3)
 ON CONFLICT(business_id,user_id) WHERE user_id IS NOT NULL DO UPDATE SET name=EXCLUDED.name
 RETURNING id`, b.ID, b.OwnerUserID, b.Name).Scan(&employee)
	if err != nil {
		return err
	}
	// A shared or unverified phone must not rename or merge existing clients.
	// A future owner-driven merge may link customers; visit snapshots stay intact.
	err = q.db.QueryRow(ctx, `INSERT INTO customers(business_id,name,phone) VALUES($1,$2,$3)
 RETURNING id`, b.ID, name, phone).Scan(&customer)
	if err != nil {
		return err
	}
	end := start.Add(time.Duration(s.DurationMinutes) * time.Minute)
	err = q.db.QueryRow(ctx, `INSERT INTO bookings(business_id,employee_id,customer_id,starts_at,ends_at,status,receipt_hash,client_name,client_phone)
 VALUES($1,$2,$3,$4,$5,'confirmed',$6,$7,$8) RETURNING id`, b.ID, employee, customer, start, end, hash, name, phone).Scan(&booking)
	if err != nil {
		return err
	}
	_, err = q.db.Exec(ctx, `INSERT INTO booking_items(booking_id,service_id,service_name,price_cents,duration_minutes,currency)
 VALUES($1,$2,$3,$4,$5,$6)`, booking, s.ID, s.Name, s.PriceCents, s.DurationMinutes, s.Currency)
	return err
}
func (q *Queries) GetReceipt(ctx context.Context, business int64, hash string) (BookingView, error) {
	var v BookingView
	err := q.db.QueryRow(ctx, `SELECT b.id,b.starts_at,b.ends_at,i.service_name,i.price_cents,i.duration_minutes,i.currency,b.status
 FROM bookings b JOIN booking_items i ON i.booking_id=b.id WHERE b.business_id=$1 AND b.receipt_hash=$2`, business, hash).
		Scan(&v.ID, &v.Start, &v.End, &v.ServiceName, &v.PriceCents, &v.Duration, &v.Currency, &v.Status)
	return v, err
}
func (q *Queries) ListOwnerBookings(ctx context.Context, owner int64) ([]BookingView, error) {
	rows, err := q.db.Query(ctx, `SELECT b.id,b.starts_at,b.ends_at,b.client_name,b.client_phone,i.service_name,i.price_cents,i.duration_minutes,i.currency,b.status
 FROM bookings b JOIN businesses p ON p.id=b.business_id
 JOIN booking_items i ON i.booking_id=b.id WHERE p.owner_user_id=$1 ORDER BY b.starts_at DESC LIMIT 200`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BookingView{}
	for rows.Next() {
		var v BookingView
		if err := rows.Scan(&v.ID, &v.Start, &v.End, &v.Name, &v.Phone, &v.ServiceName, &v.PriceCents, &v.Duration, &v.Currency, &v.Status); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
