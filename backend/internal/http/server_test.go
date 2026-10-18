package http

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestReceiptSecretsAbsentFromRequestLog(t *testing.T) {
	for _, path := range []string{"/booked/secret-token", "/booked/secret-token/extra", "/b/old-address/booked/secret-token", "/b/old-address/booked/secret-token/extra", "/b/old%2Faddress/booked/secret-token"} {
		var output bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		handler := requestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		if strings.Contains(output.String(), "secret-token") || !strings.Contains(output.String(), "[redacted]") {
			t.Fatal("receipt token entered logs", output.String())
		}
	}
}

func TestRegistrationRouteIsRateLimited(t *testing.T) {
	tmpl, err := frontend.ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, tmpl, true)
	for i := 0; i < 6; i++ {
		body := strings.NewReader("email=invalid" + string(rune('a'+i)) + "&password=supersecret")
		req := httptest.NewRequest(http.MethodPost, "/register", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "203.0.113.50:1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if i < 5 && rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d status = %d, want 400", i+1, rec.Code)
		}
		if i == 5 && (rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "") {
			t.Fatalf("request 6 status = %d Retry-After=%q", rec.Code, rec.Header().Get("Retry-After"))
		}
	}
}
