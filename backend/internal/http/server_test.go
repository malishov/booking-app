package http

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muradalishov/booking-app/frontend"
)

func TestSessionRoutes(t *testing.T) {
	tmpl, err := frontend.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, tmpl, true)
	for _, tc := range []struct {
		method, path, origin, fetchSite string
		status                          int
	}{
		{http.MethodGet, "/cabinet", "", "", http.StatusSeeOther},
		{http.MethodGet, "/cabinet/bookings", "", "", http.StatusSeeOther},
		{http.MethodGet, "/cabinet/bookings/new", "", "", http.StatusSeeOther},
		{http.MethodPost, "/cabinet/bookings/new", "", "", http.StatusSeeOther},
		{http.MethodPost, "/cabinet/bookings/1/status", "", "", http.StatusSeeOther},
		{http.MethodGet, "/cabinet/bookings/1/reschedule", "", "", http.StatusSeeOther},
		{http.MethodPost, "/cabinet/bookings/1/reschedule", "", "", http.StatusSeeOther},
		{http.MethodPost, "/cabinet/bookings/1/reschedule", "https://evil.example", "", http.StatusForbidden},
		{http.MethodPost, "/cabinet/bookings/new", "https://evil.example", "", http.StatusForbidden},
		{http.MethodPost, "/cabinet/bookings/1/status", "https://evil.example", "", http.StatusForbidden},
		{http.MethodGet, "/logout", "", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "/logout", "", "", http.StatusSeeOther},
		{http.MethodPost, "/login", "https://evil.example", "", http.StatusForbidden},
		{http.MethodPost, "/register", "https://evil.example", "", http.StatusForbidden},
		{http.MethodPost, "/logout", "https://evil.example", "", http.StatusForbidden},
		{http.MethodPost, "/logout", "", "cross-site", http.StatusForbidden},
		{http.MethodPost, "/logout", "http://example.com", "", http.StatusSeeOther},
	} {
		t.Run(tc.method+tc.path+tc.origin+tc.fetchSite, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}
}
