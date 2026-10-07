package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/muradalishov/booking-app/backend/internal/auth"
	"github.com/muradalishov/booking-app/backend/internal/store"
)

const (
	sessionCookieName = "booking_session"
	sessionLifetime   = 7 * 24 * time.Hour
)

// SessionStore is the persistence needed for session creation and authentication.
type SessionStore interface {
	CreateSession(context.Context, store.CreateSessionParams) error
	GetSessionUser(context.Context, string) (store.GetSessionUserRow, error)
	DeleteSession(context.Context, string) error
}

type sessionUserKey struct{}

// CurrentUser returns the authenticated identity set by RequireSession.
func CurrentUser(ctx context.Context) (store.GetSessionUserRow, bool) {
	user, ok := ctx.Value(sessionUserKey{}).(store.GetSessionUserRow)
	return user, ok
}

// RequireSession redirects guests and rejects revoked or expired sessions.
func (a *Auth) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		user, err := a.sessions.GetSessionUser(r.Context(), auth.SessionTokenHash(cookie.Value))
		if errors.Is(err, pgx.ErrNoRows) {
			a.clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if err != nil {
			a.serverError(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), sessionUserKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logout revokes the server-side session before clearing the browser cookie.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := a.sessions.DeleteSession(r.Context(), auth.SessionTokenHash(cookie.Value)); err != nil {
			a.serverError(w, err)
			return
		}
	}
	a.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *Auth) clearSessionCookie(w http.ResponseWriter) {
	a.setSessionCookie(w, "", time.Unix(1, 0), -1)
}

func (a *Auth) setSessionCookie(w http.ResponseWriter, token string, expires time.Time, maxAge int) {
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: a.secureCookie, SameSite: http.SameSiteLaxMode,
		Expires: expires, MaxAge: maxAge,
	})
}
