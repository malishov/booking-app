package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/muradalishov/booking-app/backend/internal/auth"
	"github.com/muradalishov/booking-app/backend/internal/store"
)

type fakeSessions struct {
	created                      store.CreateSessionParams
	user                         store.GetSessionUserRow
	deleted                      string
	createErr, getErr, deleteErr error
}

func (f *fakeSessions) CreateSession(_ context.Context, arg store.CreateSessionParams) error {
	f.created = arg
	return f.createErr
}

func (f *fakeSessions) GetSessionUser(_ context.Context, hash string) (store.GetSessionUserRow, error) {
	if f.getErr != nil {
		return store.GetSessionUserRow{}, f.getErr
	}
	if hash != f.created.TokenHash || !f.created.ExpiresAt.Time.After(time.Now()) {
		return store.GetSessionUserRow{}, pgx.ErrNoRows
	}
	return f.user, nil
}

func (f *fakeSessions) DeleteSession(_ context.Context, hash string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = hash
	if hash == f.created.TokenHash {
		f.created.TokenHash = ""
	}
	return nil
}

func TestSessionLifecycle(t *testing.T) {
	hash, err := auth.HashPassword("supersecret")
	if err != nil {
		t.Fatal(err)
	}
	h := newAuth(t, &fakeUsers{user: store.User{ID: 42, Email: "a@b.com", PasswordHash: hash}})
	sessions := h.sessions.(*fakeSessions)
	sessions.user = store.GetSessionUserRow{ID: 42, Email: "a@b.com"}
	login := httptest.NewRecorder()
	h.Login(login, postForm("email=a@b.com&password=supersecret"))
	if login.Code != http.StatusSeeOther {
		t.Fatalf("login: %d", login.Code)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d", len(cookies))
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.MaxAge != 604800 {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	if len(cookie.Value) != 43 || sessions.created.TokenHash != auth.SessionTokenHash(cookie.Value) || sessions.created.UserID != 42 {
		t.Fatal("session must store the token hash and authenticated user")
	}
	if time.Until(sessions.created.ExpiresAt.Time) < sessionLifetime-time.Minute {
		t.Fatal("incorrect lifetime")
	}
	cabinet := h.RequireSession(http.HandlerFunc(sessionIdentityPage))
	req := httptest.NewRequest(http.MethodGet, "/cabinet", nil)
	req.AddCookie(cookie)
	page := httptest.NewRecorder()
	cabinet.ServeHTTP(page, req)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "a@b.com") || page.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cabinet: %d %s", page.Code, page.Body.String())
	}
	logout := httptest.NewRecorder()
	h.Logout(logout, req)
	if logout.Code != http.StatusSeeOther || sessions.deleted != auth.SessionTokenHash(cookie.Value) {
		t.Fatal("logout did not revoke session")
	}
	if logout.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not clear cookie")
	}
	replay := httptest.NewRecorder()
	cabinet.ServeHTTP(replay, req)
	if replay.Code != http.StatusSeeOther || replay.Header().Get("Location") != "/login" {
		t.Fatal("revoked cookie still works")
	}
}

func TestRequireSessionRejectsGuests(t *testing.T) {
	for _, token := range []string{"", "unknown", "expired"} {
		t.Run(token, func(t *testing.T) {
			h := newAuth(t, &fakeUsers{})
			sessions := h.sessions.(*fakeSessions)
			sessions.created.TokenHash = auth.SessionTokenHash("expired")
			sessions.created.ExpiresAt.Time = time.Now().Add(-time.Second)
			req := httptest.NewRequest(http.MethodGet, "/cabinet", nil)
			if token != "" {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
			}
			rec := httptest.NewRecorder()
			h.RequireSession(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("guest reached protected handler") })).ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
				t.Fatal("expected login redirect")
			}
			if token != "" && rec.Result().Cookies()[0].MaxAge != -1 {
				t.Fatal("invalid cookie was not cleared")
			}
		})
	}
}

func TestSessionDatabaseFailures(t *testing.T) {
	hash, err := auth.HashPassword("supersecret")
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"create", "lookup", "delete"} {
		t.Run(operation, func(t *testing.T) {
			h := newAuth(t, &fakeUsers{user: store.User{ID: 1, PasswordHash: hash}})
			sessions := h.sessions.(*fakeSessions)
			dbErr := errors.New("database unavailable")
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/logout", nil)
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
			switch operation {
			case "create":
				sessions.createErr = dbErr
				h.Login(rec, postForm("email=a@b.com&password=supersecret"))
			case "lookup":
				sessions.getErr = dbErr
				h.RequireSession(http.HandlerFunc(sessionIdentityPage)).ServeHTTP(rec, req)
			case "delete":
				sessions.deleteErr = dbErr
				h.Logout(rec, req)
			}
			if rec.Code != http.StatusInternalServerError || len(rec.Result().Cookies()) != 0 {
				t.Fatal("DB failure must not authenticate or clear session")
			}
		})
	}
}

func sessionIdentityPage(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	_, _ = w.Write([]byte(user.Email))
}

func TestLoginRotatesExistingSession(t *testing.T) {
	hash, err := auth.HashPassword("supersecret")
	if err != nil {
		t.Fatal(err)
	}
	h := newAuth(t, &fakeUsers{user: store.User{ID: 1, PasswordHash: hash}})
	req := postForm("email=a@b.com&password=supersecret")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "previous-token"})
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	sessions := h.sessions.(*fakeSessions)
	if rec.Code != http.StatusSeeOther || sessions.deleted != auth.SessionTokenHash("previous-token") {
		t.Fatal("previous session was not revoked")
	}
	if rec.Result().Cookies()[0].Value == "previous-token" {
		t.Fatal("token was not rotated")
	}
}

func TestLocalSessionCookie(t *testing.T) {
	h := newAuth(t, &fakeUsers{})
	h.secureCookie = false
	rec := httptest.NewRecorder()
	h.setSessionCookie(rec, "token", time.Now().Add(sessionLifetime), int(sessionLifetime.Seconds()))
	cookie := rec.Result().Cookies()[0]
	if cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("local HTTP must only disable Secure")
	}
}
