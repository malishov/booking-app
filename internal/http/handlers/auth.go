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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/muradalishov/booking-app/internal/auth"
	"github.com/muradalishov/booking-app/internal/store"
)

const minPasswordLen = 8

// UserStore is the subset of the data layer the auth handlers depend on.
type UserStore interface {
	CreateUser(ctx context.Context, arg store.CreateUserParams) (store.User, error)
	GetUserByEmail(ctx context.Context, email string) (store.User, error)
}

// Auth handles registration and login.
type Auth struct {
	users  UserStore
	tmpl   *template.Template
	logger *slog.Logger
}

// NewAuth constructs an Auth handler.
func NewAuth(users UserStore, tmpl *template.Template, logger *slog.Logger) *Auth {
	return &Auth{users: users, tmpl: tmpl, logger: logger}
}

// pageData is the view model for the auth templates.
type pageData struct {
	Email string
	Error string
}

// RegisterForm renders the empty registration form.
func (a *Auth) RegisterForm(w http.ResponseWriter, _ *http.Request) {
	a.render(w, "register.html", pageData{}, http.StatusOK)
}

// Register validates the submission, creates the user, and redirects to login.
func (a *Auth) Register(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if !validEmail(email) {
		a.render(w, "register.html", pageData{Email: email, Error: "Enter a valid email address."}, http.StatusBadRequest)
		return
	}
	if len(password) < minPasswordLen {
		a.render(w, "register.html", pageData{Email: email, Error: "Password must be at least 8 characters."}, http.StatusBadRequest)
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
			a.render(w, "register.html", pageData{Email: email, Error: "That email is already registered."}, http.StatusConflict)
			return
		}
		a.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// LoginForm renders the empty login form.
func (a *Auth) LoginForm(w http.ResponseWriter, _ *http.Request) {
	a.render(w, "login.html", pageData{}, http.StatusOK)
}

// Login verifies credentials. Session handling is added in a later PR.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	invalid := func() {
		// Do not reveal whether the email or the password was wrong.
		a.render(w, "login.html", pageData{Email: email, Error: "Invalid email or password."}, http.StatusUnauthorized)
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

	a.render(w, "cabinet.html", pageData{Email: u.Email}, http.StatusOK)
}

// render executes a template into a buffer first so a template error never
// results in a half-written response.
func (a *Auth) render(w http.ResponseWriter, name string, data pageData, status int) {
	var buf bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		a.serverError(w, err)
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
