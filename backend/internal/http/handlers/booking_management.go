package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/muradalishov/booking-app/backend/internal/store"
)

func bookingFilter(value string) string {
	switch value {
	case "today", "history":
		return value
	default:
		return "upcoming"
	}
}
func bookingStatusLabel(value string) string {
	switch value {
	case "cancelled":
		return "Отменена"
	case "completed":
		return "Завершена"
	case "no_show":
		return "Не пришёл"
	case "pending":
		return "Ожидает подтверждения"
	default:
		return "Подтверждена"
	}
}
func (b *Booking) bookingProfile(r *http.Request, q *store.Queries) (store.Business, error) {
	if r.URL.Path == "/cabinet/bookings/new" {
		u, ok := CurrentUser(r.Context())
		if !ok {
			return store.Business{}, pgx.ErrNoRows
		}
		return q.GetBusinessByOwner(r.Context(), u.ID)
	}
	return q.GetBusinessBySlug(r.Context(), chi.URLParam(r, "slug"))
}
func (b *Booking) renderBooking(w http.ResponseWriter, r *http.Request, data bookingPageData, status int) {
	name := "online.html"
	if data.Manual {
		name = "booking-new.html"
	} else {
		name = "online-v2.html"
		data.EditingService = r.Method == http.MethodGet && r.URL.Query().Get("step") == "service"
		data.ServiceStep = data.Receipt == nil && (data.Selected == nil || data.EditingService || (r.Method == http.MethodGet && r.URL.Query().Get("service") == "" && data.ReviewTime == ""))
		if data.ServiceStep {
			data.ReviewTime = ""
		}

		data.PublicPath = "/b/" + data.Slug
		if strings.HasPrefix(r.URL.Path, "/v2/") {
			data.PublicPath = "/v2/b/" + data.Slug
		}
		if start, err := time.Parse(time.RFC3339, data.SelectedTime); err == nil && data.Selected != nil {
			if loc, err := time.LoadLocation(data.Timezone); err == nil {
				data.ReviewEnd = start.In(loc).Add(time.Duration(data.Selected.DurationMinutes) * time.Minute).Format("15:04")
			}
		}
		if data.Receipt != nil {
			if loc, err := time.LoadLocation(data.Timezone); err == nil {
				data.ReceiptEnd = data.Receipt.End.In(loc).Format("15:04")
				data.ReceiptShortDate = bookingShortDate(data.Receipt.Start.In(loc), requestLanguage(r))
			}
			data.ReceiptStartValue = data.Receipt.Start.UTC().Format("20060102T150405Z")
			data.ReceiptEndValue = data.Receipt.End.UTC().Format("20060102T150405Z")
		}

		data.SlotError = data.Error == bookingSlotUnavailable
		if day, err := time.Parse("2006-01-02", data.Date); err == nil {
			data.DateLabel = day.Format("02.01.2006")
		}
	}
	renderHTML(w, r, b.tmpl, b.logger, name, data, status)
}
func (b *Booking) ManualPage(w http.ResponseWriter, r *http.Request) {
	q := store.New(b.pool)
	p, err := b.bookingProfile(r, q)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Redirect(w, r, "/cabinet?setup=schedule#profile-editor", 303)
		return
	}
	if err != nil {
		b.fail(w, r, err)
		return
	}
	data, err := b.data(r, q, p)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	data.Manual = true
	u, _ := CurrentUser(r.Context())
	data.Email = u.Email
	status := 200
	if data.Error != "" {
		status = 400
	}
	b.renderBooking(w, r, data, status)
}
func (b *Booking) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", 400)
		return
	}
	status := r.PostFormValue("status")
	if status != "cancelled" && status != "completed" && status != "no_show" {
		http.Error(w, "Invalid status", 400)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	u, ok := CurrentUser(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", 303)
		return
	}
	tx, err := b.pool.Begin(r.Context())
	if err != nil {
		b.fail(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	err = store.New(tx).ChangeBookingStatus(r.Context(), u.ID, id, status, time.Now())
	returnURL := managementReturn(r.PostFormValue("filter"), r.PostFormValue("date")) + "&booking=" + strconv.FormatInt(id, 10)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Redirect(w, r, returnURL+"&error=changed", 303)
		return
	}
	if err != nil {
		b.fail(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		b.fail(w, r, err)
		return
	}
	http.Redirect(w, r, returnURL+"&saved="+status, 303)
}

func bookingShortDate(at time.Time, language string) string {
	months := []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	if language == "en" {
		months = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	}
	if language == "az" {
		months = []string{"yanvar", "fevral", "mart", "aprel", "may", "iyun", "iyul", "avqust", "sentyabr", "oktyabr", "noyabr", "dekabr"}
	}
	return strconv.Itoa(at.Day()) + " " + months[int(at.Month())-1]
}
