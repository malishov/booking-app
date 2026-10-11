package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestBusinessAliasRedirect(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/b/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if !redirectBusinessAlias(w, r, "current-address") {
			w.WriteHeader(http.StatusOK)
		}
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/b/previous-address?service=2&date=2026-10-15", nil))
	if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "/b/current-address?service=2&date=2026-10-15" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("alias lost query or cached a mutable redirect", rec)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/b/current-address", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Location") != "" {
		t.Fatal("canonical page redirected", rec)
	}
}
