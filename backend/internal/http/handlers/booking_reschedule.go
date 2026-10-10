package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/muradalishov/booking-app/backend/internal/store"
)

type reschedulePageData struct {
	bookingPageData
	Visit                                                                   store.BookingView
	Action, Back, OriginalStart, OriginalDate, OriginalTime, Price, NewDate string
}

func (b *Booking) rescheduleData(r *http.Request, q *store.Queries, profile store.Business, visit store.BookingView, now time.Time) (reschedulePageData, error) {
	loc, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return reschedulePageData{}, err
	}
	u, _ := CurrentUser(r.Context())
	d := reschedulePageData{Visit: visit, Action: fmt.Sprintf("/cabinet/bookings/%d/reschedule", visit.ID),
		Back:          managementReturn("today", visit.Start.In(loc).Format("2006-01-02")) + "&booking=" + strconv.FormatInt(visit.ID, 10),
		OriginalStart: visit.Start.UTC().Format(time.RFC3339Nano), OriginalDate: visit.Start.In(loc).Format("02.01.2006"),
		OriginalTime: visit.Start.In(loc).Format("15:04") + " — " + visit.End.In(loc).Format("15:04"), Price: fmt.Sprintf("%d.%02d", visit.PriceCents/100, visit.PriceCents%100)}
	d.Email = u.Email
	d.MinDate = now.In(loc).Format("2006-01-02")
	d.MaxDate = now.In(loc).AddDate(0, 0, bookingHorizon).Format("2006-01-02")
	d.Date = r.FormValue("date")
	if d.Date == "" {
		d.Date = visit.Start.In(loc).Format("2006-01-02")
	}
	day, parseErr := time.ParseInLocation("2006-01-02", d.Date, loc)
	if parseErr != nil || d.Date < d.MinDate || d.Date > d.MaxDate {
		d.Error = "Выберите дату в ближайшие 60 дней."
		d.Date = d.MinDate
		day, _ = time.ParseInLocation("2006-01-02", d.Date, loc)
	}
	d.TimezoneDisplay = bookingTimezoneLabel(profile.Timezone, day.Add(12*time.Hour))
	d.NewDate = day.Format("02.01.2006")
	raw, err := q.GetSchedule(r.Context(), profile.OwnerUserID)
	if err != nil {
		return d, err
	}
	settings := defaultSchedule()
	if err := json.Unmarshal(raw, &settings); err != nil {
		return d, err
	}
	first, _ := time.ParseInLocation("2006-01-02", d.MinDate, loc)
	last, _ := time.ParseInLocation("2006-01-02", d.MaxDate, loc)
	busy, err := q.ListBusyExcept(r.Context(), profile.ID, first, last.AddDate(0, 0, 1), visit.ID)
	if err != nil {
		return d, err
	}
	d.Slots = withoutCurrentSlot(availableSlots(settings, d.Date, loc, visit.Duration, busy, now), visit.Start)
	d.Calendar = bookingCalendar(settings, loc, visit.Duration, busy, now, d.Date)
	currentDate := visit.Start.In(loc).Format("2006-01-02")
	currentDayAvailable := len(withoutCurrentSlot(availableSlots(settings, currentDate, loc, visit.Duration, busy, now), visit.Start)) > 0
	for i := range d.Calendar {
		for j := range d.Calendar[i].Days {
			if d.Calendar[i].Days[j].Date == currentDate {
				d.Calendar[i].Days[j].Available = currentDayAvailable
			}
		}
	}
	return d, nil
}

func withoutCurrentSlot(slots []bookingSlot, original time.Time) []bookingSlot {
	result := []bookingSlot{}
	for _, slot := range slots {
		if slot.Value != original.UTC().Format(time.RFC3339Nano) {
			result = append(result, slot)
		}
	}
	return result
}

func (b *Booking) ReschedulePage(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	q := store.New(b.pool)
	visit, err := q.GetOwnerBooking(r.Context(), u.ID, id)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	now := time.Now()
	if visit.Status != "confirmed" || !visit.Start.After(now) {
		http.Redirect(w, r, managementReturn("history", "")+"&error=changed", http.StatusSeeOther)
		return
	}
	profile, err := q.GetBusinessByOwner(r.Context(), u.ID)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	d, err := b.rescheduleData(r, q, profile, visit, now)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if d.Error != "" {
		status = http.StatusBadRequest
	}
	renderHTML(w, r, b.tmpl, b.logger, "booking-reschedule.html", d, status)
}

func (b *Booking) Reschedule(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", 400)
		return
	}
	u, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	tx, err := b.pool.Begin(r.Context())
	if err != nil {
		b.fail(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := store.New(tx)
	profile, err := q.GetBusinessByOwner(r.Context(), u.ID)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	if err = q.LockBookingBusiness(r.Context(), profile.ID); err != nil {
		b.fail(w, r, err)
		return
	}
	profile, err = q.GetBusinessByOwner(r.Context(), u.ID)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	visit, err := q.GetOwnerBooking(r.Context(), u.ID, id)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	now := time.Now()
	if visit.Status != "confirmed" || !visit.Start.After(now) {
		http.Redirect(w, r, managementReturn("history", "")+"&error=changed", 303)
		return
	}
	d, err := b.rescheduleData(r, q, profile, visit, now)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	d.SelectedTime = r.PostFormValue("time")
	expected, parseErr := time.Parse(time.RFC3339Nano, r.PostFormValue("original_start"))
	if parseErr != nil || !expected.Equal(visit.Start) {
		d.Error = "Запись уже изменена. Проверьте текущее время и выберите новое окно."
		renderHTML(w, r, b.tmpl, b.logger, "booking-reschedule.html", d, 409)
		return
	}
	found := false
	for _, slot := range d.Slots {
		if slot.Value == d.SelectedTime {
			found = true
			break
		}
	}
	if d.Error == "" && !found {
		d.Error = "Это время уже недоступно. Выберите другое окно."
	}
	if d.Error != "" {
		renderHTML(w, r, b.tmpl, b.logger, "booking-reschedule.html", d, 400)
		return
	}
	start, _ := time.Parse(time.RFC3339, d.SelectedTime)
	err = q.RescheduleBooking(r.Context(), u.ID, id, expected, start, now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pgErr) && pgErr.Code == "23P01") {
			// A failed constraint aborts the transaction; refresh availability on
			// a healthy connection before offering another choice.
			tx.Rollback(r.Context())
			d, refreshErr := b.rescheduleData(r, store.New(b.pool), profile, visit, time.Now())
			if refreshErr != nil {
				b.fail(w, r, refreshErr)
				return
			}
			d.Error = "Это время уже недоступно. Выберите другое окно."
			renderHTML(w, r, b.tmpl, b.logger, "booking-reschedule.html", d, 409)
			return
		}
		b.fail(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		b.fail(w, r, err)
		return
	}
	loc, _ := time.LoadLocation(profile.Timezone)
	http.Redirect(w, r, managementReturn("today", start.In(loc).Format("2006-01-02"))+"&booking="+strconv.FormatInt(id, 10)+"&saved=rescheduled", 303)
}
