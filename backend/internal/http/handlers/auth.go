package handlers

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/muradalishov/booking-app/backend/internal/auth"
	"github.com/muradalishov/booking-app/backend/internal/store"
	"github.com/muradalishov/booking-app/frontend"
)

const minPasswordLen = 8

// UserStore is the subset of the data layer the auth handlers depend on.
type UserStore interface {
	CreateUser(ctx context.Context, arg store.CreateUserParams) (store.User, error)
	GetUserByEmail(ctx context.Context, email string) (store.User, error)
}

// Auth handles registration and login.
type Auth struct {
	users        UserStore
	tmpl         *template.Template
	logger       *slog.Logger
	sessions     SessionStore
	secureCookie bool
}

// NewAuth constructs an Auth handler.
func NewAuth(users UserStore, sessions SessionStore, tmpl *template.Template, logger *slog.Logger, secureCookie bool) *Auth {
	return &Auth{users: users, sessions: sessions, tmpl: tmpl, logger: logger, secureCookie: secureCookie}
}

// pageData is the view model for the auth templates.
type pageData struct {
	Email string
	Error string
}

// RegisterForm renders the empty registration form.
func (a *Auth) RegisterForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, "register.html", pageData{}, http.StatusOK)
}

// Register validates the submission, creates the user, and redirects to login.
func (a *Auth) Register(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if !validEmail(email) {
		a.render(w, r, "register.html", pageData{Email: email, Error: "Укажите корректную электронную почту."}, http.StatusBadRequest)
		return
	}
	if len(password) < minPasswordLen {
		a.render(w, r, "register.html", pageData{Email: email, Error: "Пароль должен содержать минимум 8 символов."}, http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		a.serverError(w, err)
		return
	}

	_, err = a.users.CreateUser(r.Context(), store.CreateUserParams{Email: email, PasswordHash: hash})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			a.render(w, r, "register.html", pageData{Email: email, Error: "Аккаунт с этой почтой уже существует. Попробуйте войти."}, http.StatusConflict)
			return
		}
		a.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// LoginForm renders the empty login form.
func (a *Auth) LoginForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, "login.html", pageData{}, http.StatusOK)
}

// Login verifies credentials and starts a new session.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	invalid := func() {
		// Do not reveal whether the email or the password was wrong.
		a.render(w, r, "login.html", pageData{Email: email, Error: "Неверная почта или пароль. Попробуйте ещё раз."}, http.StatusUnauthorized)
	}

	u, err := a.users.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			invalid()
			return
		}
		a.serverError(w, err)
		return
	}

	if !auth.CheckPassword(u.PasswordHash, password) {
		invalid()
		return
	}

	token, err := auth.NewSessionToken()
	if err != nil {
		a.serverError(w, err)
		return
	}
	expires := time.Now().Add(sessionLifetime)
	if err := a.sessions.CreateSession(r.Context(), store.CreateSessionParams{
		TokenHash: auth.SessionTokenHash(token), UserID: u.ID,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	}); err != nil {
		a.serverError(w, err)
		return
	}
	// Replacing an existing cookie also revokes the previous session.
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := a.sessions.DeleteSession(r.Context(), auth.SessionTokenHash(cookie.Value)); err != nil {
			a.serverError(w, err)
			return
		}
	}
	a.setSessionCookie(w, token, expires, int(sessionLifetime.Seconds()))
	http.Redirect(w, r, "/cabinet", http.StatusSeeOther)
}

// render executes a template into a buffer first so a template error never
// results in a half-written response.
func (a *Auth) render(w http.ResponseWriter, r *http.Request, name string, data any, status int) {
	renderHTML(w, r, a.tmpl, a.logger, name, data, status)
}

func renderHTML(w http.ResponseWriter, r *http.Request, tmpl *template.Template, logger *slog.Logger, name string, data any, status int) {
	w.Header().Set("Cache-Control", "no-store")
	language := requestLanguage(r)
	w.Header().Set("Content-Language", language)
	path := r.URL.RequestURI()
	if r.URL.Path == "/cabinet/business" {
		path = "/cabinet"
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/cabinet/services/") {
		path = "/cabinet/services"
	}
	localized, err := frontend.LocalizedTemplates(tmpl, language, path)
	if err != nil {
		logger.Error("localize templates", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := localized.ExecuteTemplate(&buf, name, data); err != nil {
		logger.Error("template render error", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (a *Auth) serverError(w http.ResponseWriter, err error) {
	a.logger.Error("auth handler error", "err", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func validEmail(s string) bool {
	if s == "" {
		return false
	}
	_, err := mail.ParseAddress(s)
	return err == nil
}
