package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/muradalishov/booking-app/backend/internal/auth"
	"github.com/muradalishov/booking-app/backend/internal/store"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Booking struct {
	pool   *pgxpool.Pool
	tmpl   *template.Template
	logger *slog.Logger
}

func NewBooking(p *pgxpool.Pool, t *template.Template, l *slog.Logger) *Booking {
	return &Booking{p, t, l}
}

type bookingPageData struct {
	businessForm
	Manual                                                         bool
	Email                                                          string
	TimezoneDisplay                                                string
	Services                                                       []serviceView
	SelectedID                                                     int64
	Date, MinDate, MaxDate, Error, ClientName, Phone, SelectedTime string
	Slots                                                          []bookingSlot
	Calendar                                                       []calendarMonth
	Selected                                                       *serviceView
	Receipt                                                        *store.BookingView
	ReceiptDate, ReceiptTime, ReceiptPrice                         string
}

func (b *Booking) data(r *http.Request, q *store.Queries, profile store.Business) (bookingPageData, error) {
	data := bookingPageData{businessForm: businessForm{Name: profile.Name, Slug: profile.Slug, Description: profile.Description, Timezone: profile.Timezone}}
	loc, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return data, err
	}
	now := time.Now()
	data.TimezoneDisplay = bookingTimezoneLabel(profile.Timezone, now.In(loc))
	data.MinDate = now.In(loc).Format("2006-01-02")
	data.MaxDate = now.In(loc).AddDate(0, 0, bookingHorizon).Format("2006-01-02")
	data.Date = r.FormValue("date")
	if data.Date == "" {
		data.Date = data.MinDate
	}
	day, err := time.ParseInLocation("2006-01-02", data.Date, loc)
	if err == nil {
		data.TimezoneDisplay = bookingTimezoneLabel(profile.Timezone, day.Add(12*time.Hour))
	}
	if err != nil || data.Date < data.MinDate || data.Date > data.MaxDate {
		data.Error = "Выберите дату в ближайшие 60 дней."
		data.Date = data.MinDate
		day, _ = time.ParseInLocation("2006-01-02", data.Date, loc)
	}
	services, err := q.ListPublicServices(r.Context(), profile.ID)
	if err != nil {
		return data, err
	}
	data.Services = serviceViews(services)
	rawID := r.FormValue("service")
	data.SelectedID, _ = strconv.ParseInt(rawID, 10, 64)
	if rawID == "" && len(data.Services) > 0 {
		data.SelectedID = data.Services[0].ID
	}
	for _, s := range data.Services {
		if s.ID == data.SelectedID {
			copy := s
			data.Selected = &copy
			break
		}
	}
	if data.Selected == nil {
		if rawID != "" {
			data.Error = "Эта услуга недоступна. Выберите другую."
		}
		return data, nil
	}
	raw, err := q.GetSchedule(r.Context(), profile.OwnerUserID)
	if err != nil {
		return data, err
	}
	settings := defaultSchedule()
	if err = json.Unmarshal(raw, &settings); err != nil {
		return data, err
	}
	first, _ := time.ParseInLocation("2006-01-02", data.MinDate, loc)
	last, _ := time.ParseInLocation("2006-01-02", data.MaxDate, loc)
	busy, err := q.ListBusy(r.Context(), profile.ID, first, last.AddDate(0, 0, 1))
	if err != nil {
		return data, err
	}
	data.Slots = availableSlots(settings, data.Date, loc, data.Selected.DurationMinutes, busy, now)
	data.Calendar = bookingCalendar(settings, loc, data.Selected.DurationMinutes, busy, now, data.Date)
	// Start on the first bookable day for an initial visit, rather than an
	// already closed today. Explicit date choices stay unchanged.
	if r.Method == http.MethodGet && r.FormValue("date") == "" {
		chosen := ""
		for _, month := range data.Calendar {
			for _, cell := range month.Days {
				if cell.Available {
					chosen = cell.Date
					break
				}
			}
			if chosen != "" {
				break
			}
		}
		if chosen != "" {
			data.Date = chosen
			selected, _ := time.ParseInLocation("2006-01-02", chosen, loc)
			data.TimezoneDisplay = bookingTimezoneLabel(profile.Timezone, selected.Add(12*time.Hour))
			data.Slots = availableSlots(settings, chosen, loc, data.Selected.DurationMinutes, busy, now)
			for i := range data.Calendar {
				data.Calendar[i].Selected = false
				for j := range data.Calendar[i].Days {
					cell := &data.Calendar[i].Days[j]
					cell.Selected = cell.Date == chosen
					if cell.Selected {
						data.Calendar[i].Selected = true
					}
				}
			}
		}
	}
	return data, nil
}
func (b *Booking) Page(w http.ResponseWriter, r *http.Request) {
	q := store.New(b.pool)
	p, err := q.GetBusinessBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		b.fail(w, r, err)
		return
	}
	if redirectBusinessAlias(w, r, p.Slug) {
		return
	}
	data, err := b.data(r, q, p)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	status := 200
	if data.Error != "" {
		status = 400
	}
	renderHTML(w, r, b.tmpl, b.logger, "online.html", data, status)
}
func normalizeClient(name, phone string) (string, string, string) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 100 {
		return name, phone, "Введите имя от 2 до 100 символов."
	}
	phone = strings.TrimSpace(phone)
	var normalized strings.Builder
	for i, c := range phone {
		if c >= '0' && c <= '9' {
			normalized.WriteRune(c)
		} else if c == '+' && i == 0 {
			normalized.WriteRune(c)
		} else if c != ' ' && c != '-' && c != '(' && c != ')' {
			return name, phone, "Введите телефон: от 7 до 15 цифр, можно с кодом страны."
		}
	}
	value := normalized.String()
	digits := strings.TrimPrefix(value, "+")
	if len(digits) < 7 || len(digits) > 15 {
		return name, phone, "Введите телефон: от 7 до 15 цифр, можно с кодом страны."
	}
	return name, value, ""
}
func (b *Booking) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", 400)
		return
	}
	tx, err := b.pool.Begin(r.Context())
	if err != nil {
		b.fail(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := store.New(tx)
	profile, err := b.bookingProfile(r, q)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	if err = q.LockBookingBusiness(r.Context(), profile.ID); err != nil {
		b.fail(w, r, err)
		return
	}
	profile, err = b.bookingProfile(r, q)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	// Re-read all availability inside the transaction after obtaining the lock.
	data, err := b.data(r, q, profile)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	if r.URL.Path == "/cabinet/bookings/new" {
		data.Manual = true
		u, _ := CurrentUser(r.Context())
		data.Email = u.Email
	}
	name, phone, message := normalizeClient(r.PostFormValue("client_name"), r.PostFormValue("phone"))
	data.ClientName = name
	data.Phone = r.PostFormValue("phone")
	data.SelectedTime = r.PostFormValue("time")
	if data.Error != "" {
		message = data.Error
	}
	found := false
	for _, slot := range data.Slots {
		if slot.Value == data.SelectedTime {
			found = true
			break
		}
	}
	if message == "" && !found {
		message = "Это время уже недоступно. Выберите другое окно."
	}
	if message != "" {
		data.Error = message
		b.renderBooking(w, r, data, 400)
		return
	}
	services, err := q.ListPublicServices(r.Context(), profile.ID)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	var service store.Service
	for _, s := range services {
		if s.ID == data.SelectedID {
			service = s
			break
		}
	}
	if service.ID == 0 {
		data.Error = "Эта услуга недоступна. Выберите другую."
		b.renderBooking(w, r, data, 409)
		return
	}
	token, err := auth.NewSessionToken()
	if err != nil {
		b.fail(w, r, err)
		return
	}
	start, _ := time.Parse(time.RFC3339, data.SelectedTime)
	err = q.InsertBooking(r.Context(), profile, service, name, phone, auth.SessionTokenHash(token), start)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23P01" {
			_ = tx.Rollback(r.Context())
			data.Error = "Это время уже недоступно. Выберите другое окно."
			data.Slots = nil
			b.renderBooking(w, r, data, 409)
			return
		}
		b.fail(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		b.fail(w, r, err)
		return
	}
	if data.Manual {
		http.Redirect(w, r, "/cabinet/bookings?filter=upcoming&saved=created", 303)
		return
	}
	http.Redirect(w, r, "/booked/"+token, 303)
}
func (b *Booking) Receipt(w http.ResponseWriter, r *http.Request) {
	q := store.New(b.pool)
	token := chi.URLParam(r, "token")
	hash := auth.SessionTokenHash(token)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "private, no-store")
	p, err := q.GetReceiptBusiness(r.Context(), pgtype.Text{String: hash, Valid: true})
	if err != nil {
		b.fail(w, r, err)
		return
	}
	if slug := chi.URLParam(r, "slug"); slug != "" {
		// The validated secret identifies the visit. Even a slug lost before
		// aliases were introduced (or subsequently reused) cannot hijack it.
		http.Redirect(w, r, "/booked/"+token, http.StatusFound)
		return
	}
	receipt, err := q.GetReceipt(r.Context(), p.ID, hash)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	data := bookingPageData{businessForm: businessForm{Name: p.Name, Slug: p.Slug, Timezone: p.Timezone}, Receipt: &receipt, ReceiptDate: receipt.Start.In(loc).Format("02.01.2006"), ReceiptTime: receipt.Start.In(loc).Format("15:04"), ReceiptPrice: fmt.Sprintf("%d.%02d", receipt.PriceCents/100, receipt.PriceCents%100)}
	data.TimezoneDisplay = bookingTimezoneLabel(p.Timezone, receipt.Start.In(loc))
	w.Header().Set("Referrer-Policy", "no-referrer")
	renderHTML(w, r, b.tmpl, b.logger, "online.html", data, 200)
}

type ownerBookingView struct {
	store.BookingView
	Date, Time, Price                 string
	CanCancel, CanComplete, CanNoShow bool
	CanReschedule                     bool
	StatusLabel                       string
	ReturnFilter, ReturnDate          string
	Selected                          bool
}

func (b *Booking) Cabinet(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", 303)
		return
	}
	q := store.New(b.pool)
	p, err := q.GetBusinessByOwner(r.Context(), u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Redirect(w, r, "/cabinet?setup=schedule#profile-editor", 303)
		return
	}
	if err != nil {
		b.fail(w, r, err)
		return
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		b.fail(w, r, err)
		return
	}
	now := time.Now()
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	filter := bookingFilter(r.FormValue("filter"))
	if r.FormValue("filter") == "" {
		filter = "today"
	}
	page := ownerBookingsPage{Email: u.Email, Business: p, Filter: filter, Saved: r.URL.Query().Get("saved"), Error: r.URL.Query().Get("error") == "changed"}
	var rows []store.BookingView
	if filter == "today" {
		day := today
		if raw := r.URL.Query().Get("date"); raw != "" {
			parsed, e := time.ParseInLocation("2006-01-02", raw, loc)
			if e != nil || parsed.Format("2006-01-02") != raw {
				page.ErrorMessage = "Укажите корректную дату."
			} else {
				day = parsed
			}
		}
		rows, err = q.ListOwnerDayBookings(r.Context(), u.ID, day, day.AddDate(0, 0, 1))
		if err != nil {
			b.fail(w, r, err)
			return
		}
		raw, err := q.GetSchedule(r.Context(), u.ID)
		if err != nil {
			b.fail(w, r, err)
			return
		}
		settings := defaultSchedule()
		if err = json.Unmarshal(raw, &settings); err != nil {
			b.fail(w, r, err)
			return
		}
		page.Calendar = buildOwnerCalendar(day, settings, rows, now)
	} else {
		rows, err = q.ListManagedBookings(r.Context(), u.ID, filter, now, today, today.AddDate(0, 0, 1))
	}
	if err != nil {
		b.fail(w, r, err)
		return
	}
	page.Bookings = ownerBookingViews(rows, loc, now)
	page.SelectedID = selectedBooking(r.URL.Query().Get("booking"), rows)
	for i := range page.Bookings {
		page.Bookings[i].ReturnFilter = filter
		page.Bookings[i].ReturnDate = page.Calendar.Date
		page.Bookings[i].Selected = page.Bookings[i].ID == page.SelectedID
	}
	status := 200
	if page.ErrorMessage != "" {
		status = 400
	}
	renderHTML(w, r, b.tmpl, b.logger, "bookings.html", page, status)
}
func (b *Booking) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		renderHTML(w, r, b.tmpl, b.logger, "not-found.html", nil, 404)
		return
	}
	b.logger.Error("booking failed", "error", err)
	http.Error(w, "Internal server error", 500)
}

func bookingTimezoneLabel(zone string, at time.Time) string {
	_, offset := at.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	label := fmt.Sprintf("%s — UTC%s%d", zone, sign, offset/3600)
	if minutes := offset / 60 % 60; minutes != 0 {
		label += fmt.Sprintf(":%02d", minutes)
	}
	return label
}
