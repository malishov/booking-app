package http

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestRateLimiterRefillsAndBoundsStorage(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	limiter := newRateLimiter(2)
	limiter.now = func() time.Time { return now }
	policy := ratePolicy{capacity: 2, period: time.Minute}

	if ok, _ := limiter.allow("first", policy); !ok {
		t.Fatal("first request rejected")
	}
	if ok, _ := limiter.allow("first", policy); !ok {
		t.Fatal("second request rejected")
	}
	if ok, retry := limiter.allow("first", policy); ok || retry != 30*time.Second {
		t.Fatalf("third request = %v, retry %v; want rejected for 30s", ok, retry)
	}

	now = now.Add(30 * time.Second)
	if ok, _ := limiter.allow("first", policy); !ok {
		t.Fatal("request was not admitted after token refill")
	}
	limiter.allow("second", policy)
	limiter.allow("third", policy)
	if len(limiter.buckets) != 2 {
		t.Fatalf("bucket count = %d, want bounded to 2", len(limiter.buckets))
	}
}

func TestLimitRequestsReturns429AndIgnoresForwardedFor(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	limiter := newRateLimiter(100)
	limiter.now = func() time.Time { return now }
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var calls atomic.Int32
	handler := limitRequests(limiter, logger, "login", ratePolicy{capacity: 1, period: time.Minute}, ipKey)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))

	request := func(forwarded string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(""))
		req.RemoteAddr = "203.0.113.10:4321"
		req.Header.Set("X-Forwarded-For", forwarded)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if got := request("198.51.100.1").Code; got != http.StatusNoContent {
		t.Fatalf("first status = %d", got)
	}
	rec := request("198.51.100.2")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("limited response = %d Retry-After=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if calls.Load() != 1 {
		t.Fatalf("downstream calls = %d, want 1", calls.Load())
	}
}

func TestLimitRequestsConcurrentCapacity(t *testing.T) {
	limiter := newRateLimiter(100)
	policy := ratePolicy{capacity: 5, period: time.Hour}
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := limiter.allow("same-key", policy); ok {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 5 {
		t.Fatalf("admitted = %d, want exactly 5", admitted.Load())
	}
}

func TestAuthLimitFollowsHashedEmailAcrossIPs(t *testing.T) {
	limiter := newRateLimiter(100)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var calls atomic.Int32
	handler := limitAuthRequests(limiter, logger, "login", ratePolicy{capacity: 10, period: time.Hour}, ratePolicy{capacity: 1, period: time.Hour})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))

	request := func(ip, email string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("email="+email+"&password=secret"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if got := request("203.0.113.1", "USER%40example.com").Code; got != http.StatusNoContent {
		t.Fatalf("first status = %d", got)
	}
	if got := request("203.0.113.2", "user%40example.com").Code; got != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("downstream calls = %d, want 1", calls.Load())
	}
}

func TestRateLimitResponseIsLocalized(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.AddCookie(&http.Cookie{Name: "ui_language", Value: "en"})
	rec := httptest.NewRecorder()
	rateLimitResponse(rec, req, 12)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "12" || !strings.Contains(rec.Body.String(), "Too many attempts") {
		t.Fatalf("unexpected response: status=%d headers=%v body=%q", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestBookingRateKeyIncludesResolvedSlugAndRemoteIP(t *testing.T) {
	router := chi.NewRouter()
	router.Post("/b/{slug}/book", func(w http.ResponseWriter, r *http.Request) {
		if got := businessIPKey(r); got != "anna-nails:203.0.113.20" {
			t.Fatalf("key = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/b/anna-nails/book", nil)
	req.RemoteAddr = "203.0.113.20:1234"
	router.ServeHTTP(httptest.NewRecorder(), req)
}
