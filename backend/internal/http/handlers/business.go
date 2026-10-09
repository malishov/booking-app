package handlers

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/muradalishov/booking-app/backend/internal/store"
	"github.com/muradalishov/booking-app/frontend"
	// Embed IANA timezone rules for the minimal Docker runtime.
	_ "time/tzdata"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var profileTimezones = []string{
	"Asia/Dubai", "Europe/Moscow", "Europe/Kyiv", "Europe/Minsk",
	"Asia/Baku", "Asia/Tbilisi", "Asia/Yerevan", "Asia/Almaty",
	"Asia/Tashkent", "Europe/Berlin", "Europe/London", "America/New_York", "UTC",
}

// BusinessStore persists profiles scoped to their authenticated owner.
type BusinessStore interface {
	GetBusinessByOwner(context.Context, int64) (store.Business, error)
	GetBusinessBySlug(context.Context, string) (store.Business, error)
	SaveBusiness(context.Context, store.SaveBusinessParams) (store.Business, error)
	ListPublicServices(context.Context, int64) ([]store.Service, error)
}

// Business handles the master's profile and public page.
type Business struct {
	store  BusinessStore
	tmpl   *template.Template
	logger *slog.Logger
}

// NewBusiness constructs business profile handlers.
func NewBusiness(s BusinessStore, tmpl *template.Template, logger *slog.Logger) *Business {
	return &Business{store: s, tmpl: tmpl, logger: logger}
}

type businessForm struct{ Name, Slug, Description, Timezone string }
type publicBusinessPageData struct {
	businessForm
	Services []serviceView
}

type businessPageData struct {
	Email         string
	Business      *store.Business
	Form          businessForm
	Timezones     []string
	Error         string
	Saved         bool
	NeedsBusiness bool
}

// Cabinet renders the profile editor and its current public link.
func (b *Business) Cabinet(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	data := businessPageData{Email: user.Email, Form: businessForm{Timezone: "UTC"}, Saved: r.URL.Query().Get("saved") == "1", NeedsBusiness: r.URL.Query().Get("setup") == "services" || r.URL.Query().Get("setup") == "schedule"}
	profile, err := b.store.GetBusinessByOwner(r.Context(), user.ID)
	if err == nil {
		data.Business = &profile
		data.Form = businessForm{Name: profile.Name, Slug: profile.Slug, Description: profile.Description, Timezone: profile.Timezone}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		b.serverError(w, err)
		return
	}
	b.renderCabinet(w, r, data, http.StatusOK)
}

// SaveProfile creates or updates only the authenticated owner's business.
func (b *Business) SaveProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, frontend.Translate(requestLanguage(r), "Форма слишком большая или некорректная."), http.StatusBadRequest)
		return
	}
	form := businessForm{
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Slug:        strings.ToLower(strings.TrimSpace(r.PostFormValue("slug"))),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Timezone:    strings.TrimSpace(r.PostFormValue("timezone")),
	}
	data := businessPageData{Email: user.Email, Form: form}
	// Keep the existing public link visible when validation fails.
	existing, err := b.store.GetBusinessByOwner(r.Context(), user.ID)
	if err == nil {
		data.Business = &existing
	} else if !errors.Is(err, pgx.ErrNoRows) {
		b.serverError(w, err)
		return
	}
	if message := validateBusiness(form); message != "" {
		data.Error = message
		b.renderCabinet(w, r, data, http.StatusBadRequest)
		return
	}
	_, err = b.store.SaveBusiness(r.Context(), store.SaveBusinessParams{
		OwnerUserID: user.ID, Name: form.Name, Slug: form.Slug, Description: form.Description, Timezone: form.Timezone,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "businesses_slug_key" {
			data.Error = "Этот адрес уже занят. Попробуйте другой."
			b.renderCabinet(w, r, data, http.StatusConflict)
			return
		}
		b.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/cabinet?saved=1", http.StatusSeeOther)
}

// PublicPage shows a profile to clients without exposing the owner's identity.
func (b *Business) PublicPage(w http.ResponseWriter, r *http.Request) {
	profile, err := b.store.GetBusinessBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, pgx.ErrNoRows) {
		renderHTML(w, r, b.tmpl, b.logger, "not-found.html", nil, http.StatusNotFound)
		return
	}
	if err != nil {
		b.serverError(w, err)
		return
	}
	// Use an explicit public view model: never pass account or owner data.
	services, err := b.store.ListPublicServices(r.Context(), profile.ID)
	if err != nil {
		b.serverError(w, err)
		return
	}
	renderHTML(w, r, b.tmpl, b.logger, "business.html", publicBusinessPageData{businessForm: businessForm{
		Name: profile.Name, Slug: profile.Slug, Description: profile.Description, Timezone: profile.Timezone,
	}, Services: serviceViews(services)}, http.StatusOK)
}

func (b *Business) renderCabinet(w http.ResponseWriter, r *http.Request, data businessPageData, status int) {
	data.Timezones = append([]string(nil), profileTimezones...)
	found := false
	for _, zone := range data.Timezones {
		if zone == data.Form.Timezone {
			found = true
			break
		}
	}
	if !found && data.Form.Timezone != "" {
		data.Timezones = append(data.Timezones, data.Form.Timezone)
	}
	renderHTML(w, r, b.tmpl, b.logger, "cabinet.html", data, status)
}

func (b *Business) serverError(w http.ResponseWriter, err error) {
	b.logger.Error("business handler error", "err", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func validateBusiness(form businessForm) string {
	if utf8.RuneCountInString(form.Name) < 2 || utf8.RuneCountInString(form.Name) > 100 {
		return "Название должно содержать от 2 до 100 символов."
	}
	if len(form.Slug) < 3 || len(form.Slug) > 60 || !slugPattern.MatchString(form.Slug) {
		return "Адрес: от 3 до 60 символов, латинские буквы, цифры и дефисы между словами."
	}
	if utf8.RuneCountInString(form.Description) > 1000 {
		return "Описание не должно превышать 1000 символов."
	}
	if form.Timezone == "" || form.Timezone == "Local" {
		return "Выберите часовой пояс из базы IANA, например Asia/Dubai."
	}
	if _, err := time.LoadLocation(form.Timezone); err != nil {
		return "Укажите корректный часовой пояс, например Asia/Dubai."
	}
	return ""
}
