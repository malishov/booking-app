package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/muradalishov/booking-app/internal/auth"
	"github.com/muradalishov/booking-app/internal/store"
	"github.com/muradalishov/booking-app/web"
)

type fakeUsers struct {
	createErr    error
	created      store.CreateUserParams
	createCalled bool
	user         store.User
	getErr       error
}

func (f *fakeUsers) CreateUser(_ context.Context, arg store.CreateUserParams) (store.User, error) {
	f.createCalled = true
	f.created = arg
	if f.createErr != nil {
		return store.User{}, f.createErr
	}
	return f.user, nil
}

func (f *fakeUsers) GetUserByEmail(_ context.Context, _ string) (store.User, error) {
	if f.getErr != nil {
		return store.User{}, f.getErr
	}
	return f.user, nil
}

func newAuth(t *testing.T, users UserStore) *Auth {
	t.Helper()
	tmpl, err := web.ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	return NewAuth(users, tmpl, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func postForm(values string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestRegister_Success(t *testing.T) {
	users := &fakeUsers{}
	h := newAuth(t, users)

	rec := httptest.NewRecorder()
	h.Register(rec, postForm("email=a@b.com&password=supersecret"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/login" {
		t.Fatalf("redirect = %q, want /login", got)
	}
	if !users.createCalled {
		t.Fatal("CreateUser was not called")
	}
	if users.created.PasswordHash == "supersecret" {
		t.Fatal("password was stored in plaintext")
	}
	if !auth.CheckPassword(users.created.PasswordHash, "supersecret") {
		t.Fatal("stored hash does not verify against the password")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	users := &fakeUsers{createErr: &pgconn.PgError{Code: "23505"}}
	h := newAuth(t, users)

	rec := httptest.NewRecorder()
	h.Register(rec, postForm("email=a@b.com&password=supersecret"))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if !strings.Contains(rec.Body.String(), "already registered") {
		t.Fatal("expected duplicate-email message in response")
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	users := &fakeUsers{}
	h := newAuth(t, users)

	rec := httptest.NewRecorder()
	h.Register(rec, postForm("email=not-an-email&password=supersecret"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if users.createCalled {
		t.Fatal("CreateUser must not be called for invalid input")
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	users := &fakeUsers{}
	h := newAuth(t, users)

	rec := httptest.NewRecorder()
	h.Register(rec, postForm("email=a@b.com&password=short"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if users.createCalled {
		t.Fatal("CreateUser must not be called for a short password")
	}
}

func TestLogin_Success(t *testing.T) {
	hash, _ := auth.HashPassword("supersecret")
	users := &fakeUsers{user: store.User{Email: "a@b.com", PasswordHash: hash}}
	h := newAuth(t, users)

	rec := httptest.NewRecorder()
	h.Login(rec, postForm("email=a@b.com&password=supersecret"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Welcome") {
		t.Fatal("expected cabinet page on successful login")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	hash, _ := auth.HashPassword("supersecret")
	users := &fakeUsers{user: store.User{Email: "a@b.com", PasswordHash: hash}}
	h := newAuth(t, users)

	rec := httptest.NewRecorder()
	h.Login(rec, postForm("email=a@b.com&password=wrongpassword"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestLogin_NoSuchUser(t *testing.T) {
	users := &fakeUsers{getErr: pgx.ErrNoRows}
	h := newAuth(t, users)

	rec := httptest.NewRecorder()
	h.Login(rec, postForm("email=ghost@b.com&password=supersecret"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
