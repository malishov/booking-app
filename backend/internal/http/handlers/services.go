package handlers

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/muradalishov/booking-app/backend/internal/store"
	"github.com/muradalishov/booking-app/frontend"
)

var pricePattern = regexp.MustCompile(`^[0-9]{1,7}(?:[.,][0-9]{1,2})?$`)
var serviceCurrencies = []string{"AZN", "AED", "RUB", "USD", "EUR", "GBP"}

// ServiceStore always scopes private operations to an authenticated owner.
type ServiceStore interface {
	GetBusinessByOwner(context.Context, int64) (store.Business, error)
	ListServicesByOwner(context.Context, int64) ([]store.Service, error)
	GetServiceByOwner(context.Context, store.GetServiceByOwnerParams) (store.Service, error)
	CreateService(context.Context, store.CreateServiceParams) (store.Service, error)
	UpdateService(context.Context, store.UpdateServiceParams) (store.Service, error)
	ToggleService(context.Context, store.ToggleServiceParams) (store.Service, error)
}

// Services manages a master's service catalog.
type Services struct {
	store  ServiceStore
	tmpl   *template.Template
	logger *slog.Logger
}

// NewServices constructs service catalog handlers.
func NewServices(s ServiceStore, tmpl *template.Template, logger *slog.Logger) *Services {
	return &Services{store: s, tmpl: tmpl, logger: logger}
}

type serviceForm struct{ Name, Duration, Price, Currency string }
type serviceView struct {
	ID              int64
	Name            string
	DurationMinutes int32
	Price, Currency string
	IsActive        bool
}
type servicePageData struct {
	Email      string
	Business   store.Business
	Form       serviceForm
	EditingID  int64
	Services   []serviceView
	Currencies []string
	Error      string
	Saved      bool
}

func serviceViews(rows []store.Service) []serviceView {
	views := make([]serviceView, 0, len(rows))
	for _, row := range rows {
		views = append(views, serviceView{ID: row.ID, Name: row.Name, DurationMinutes: row.DurationMinutes, Price: fmt.Sprintf("%d.%02d", row.PriceCents/100, row.PriceCents%100), Currency: row.Currency, IsActive: row.IsActive})
	}
	return views
}

func (s *Services) ownerProfile(w http.ResponseWriter, r *http.Request) (store.Business, bool) {
	user, ok := CurrentUser(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return store.Business{}, false
	}
	profile, err := s.store.GetBusinessByOwner(r.Context(), user.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Redirect(w, r, "/cabinet?setup=services#profile-editor", http.StatusSeeOther)
		return store.Business{}, false
	}
	if err != nil {
		s.fail(w, err)
		return store.Business{}, false
	}
	return profile, true
}

// Page lists services and loads an optional editor for an owned service.
func (s *Services) Page(w http.ResponseWriter, r *http.Request) {
	profile, ok := s.ownerProfile(w, r)
	if !ok {
		return
	}
	form := serviceForm{Duration: "60", Price: "", Currency: "AZN"}
	var editingID int64
	if raw := r.URL.Query().Get("edit"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}
		row, err := s.store.GetServiceByOwner(r.Context(), store.GetServiceByOwnerParams{ID: id, OwnerUserID: profile.OwnerUserID})
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		editingID = row.ID
		form = serviceForm{Name: row.Name, Duration: strconv.Itoa(int(row.DurationMinutes)), Price: serviceViews([]store.Service{row})[0].Price, Currency: row.Currency}
	}
	s.render(w, r, profile, form, editingID, "", http.StatusOK)
}

// Save creates or edits a service without trusting client-supplied ownership.
func (s *Services) Save(w http.ResponseWriter, r *http.Request) {
	profile, ok := s.ownerProfile(w, r)
	if !ok {
		return
	}
	var editingID int64
	if raw := chi.URLParam(r, "id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}
		_, err = s.store.GetServiceByOwner(r.Context(), store.GetServiceByOwnerParams{ID: id, OwnerUserID: profile.OwnerUserID})
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		editingID = id
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, frontend.Translate(requestLanguage(r), "Форма слишком большая или некорректная."), http.StatusBadRequest)
		return
	}
	form := serviceForm{Name: strings.TrimSpace(r.PostFormValue("name")), Duration: strings.TrimSpace(r.PostFormValue("duration")), Price: strings.TrimSpace(r.PostFormValue("price")), Currency: r.PostFormValue("currency")}
	duration, cents, message := validateService(form)
	if message != "" {
		s.render(w, r, profile, form, editingID, message, http.StatusBadRequest)
		return
	}
	var err error
	if editingID == 0 {
		_, err = s.store.CreateService(r.Context(), store.CreateServiceParams{OwnerUserID: profile.OwnerUserID, Name: form.Name, DurationMinutes: duration, PriceCents: cents, Currency: form.Currency})
	} else {
		_, err = s.store.UpdateService(r.Context(), store.UpdateServiceParams{ID: editingID, OwnerUserID: profile.OwnerUserID, Name: form.Name, DurationMinutes: duration, PriceCents: cents, Currency: form.Currency})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, "/cabinet/services?saved=1#service-list", http.StatusSeeOther)
}

// Toggle reversibly hides or restores an owned service.
func (s *Services) Toggle(w http.ResponseWriter, r *http.Request) {
	profile, ok := s.ownerProfile(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	_, err = s.store.ToggleService(r.Context(), store.ToggleServiceParams{ID: id, OwnerUserID: profile.OwnerUserID})
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, "/cabinet/services?saved=1#service-list", http.StatusSeeOther)
}

func (s *Services) render(w http.ResponseWriter, r *http.Request, profile store.Business, form serviceForm, id int64, message string, status int) {
	rows, err := s.store.ListServicesByOwner(r.Context(), profile.OwnerUserID)
	if err != nil {
		s.fail(w, err)
		return
	}
	user, _ := CurrentUser(r.Context())
	renderHTML(w, r, s.tmpl, s.logger, "services.html", servicePageData{Email: user.Email, Business: profile, Form: form, EditingID: id, Services: serviceViews(rows), Currencies: serviceCurrencies, Error: message, Saved: r.URL.Query().Get("saved") == "1"}, status)
}

func (s *Services) fail(w http.ResponseWriter, err error) {
	s.logger.Error("service handler error", "err", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func validateService(form serviceForm) (int32, int32, string) {
	if n := utf8.RuneCountInString(form.Name); n < 2 || n > 100 {
		return 0, 0, "Название должно содержать от 2 до 100 символов."
	}
	duration, err := strconv.ParseInt(form.Duration, 10, 32)
	if err != nil || duration < 1 || duration > 1440 {
		return 0, 0, "Длительность: целое число от 1 до 1440 минут."
	}
	if !pricePattern.MatchString(form.Price) {
		return 0, 0, "Цена: от 0 до 9999999.99, не более двух знаков после запятой."
	}
	parts := strings.Split(strings.ReplaceAll(form.Price, ",", "."), ".")
	units, _ := strconv.ParseInt(parts[0], 10, 32)
	var fraction int64
	if len(parts) == 2 {
		fraction, _ = strconv.ParseInt(parts[1]+strings.Repeat("0", 2-len(parts[1])), 10, 32)
	}
	supported := false
	for _, currency := range serviceCurrencies {
		if form.Currency == currency {
			supported = true
			break
		}
	}
	if !supported {
		return 0, 0, "Выберите поддерживаемую валюту."
	}
	return int32(duration), int32(units*100 + fraction), ""
}
