package http

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muradalishov/booking-app/frontend"
)

type ratePolicy struct {
	capacity int
	period   time.Duration
}

type rateBucket struct {
	tokens   float64
	updated  time.Time
	lastSeen time.Time
}

// rateLimiter is a bounded in-memory token bucket store. It protects one app
// instance; deployments with multiple replicas need a shared limiter backend.
type rateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]rateBucket
	maxEntries int
	now        func() time.Time
}

func newRateLimiter(maxEntries int) *rateLimiter {
	return &rateLimiter{buckets: make(map[string]rateBucket), maxEntries: maxEntries, now: time.Now}
}

func (l *rateLimiter) allow(key string, policy ratePolicy) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) >= l.maxEntries {
		l.removeStale(now, policy.period*2)
	}
	if len(l.buckets) >= l.maxEntries {
		l.removeOldest()
	}

	bucket, ok := l.buckets[key]
	if !ok {
		bucket = rateBucket{tokens: float64(policy.capacity), updated: now}
	}
	elapsed := now.Sub(bucket.updated)
	if elapsed > 0 {
		bucket.tokens = math.Min(float64(policy.capacity), bucket.tokens+elapsed.Seconds()*float64(policy.capacity)/policy.period.Seconds())
		bucket.updated = now
	}
	bucket.lastSeen = now
	if bucket.tokens >= 1 {
		bucket.tokens--
		l.buckets[key] = bucket
		return true, 0
	}

	l.buckets[key] = bucket
	waitSeconds := (1 - bucket.tokens) * policy.period.Seconds() / float64(policy.capacity)
	return false, time.Duration(math.Ceil(waitSeconds)) * time.Second
}

func (l *rateLimiter) removeStale(now time.Time, ttl time.Duration) {
	for key, bucket := range l.buckets {
		if now.Sub(bucket.lastSeen) > ttl {
			delete(l.buckets, key)
		}
	}
}

func (l *rateLimiter) removeOldest() {
	var oldestKey string
	var oldest time.Time
	for key, bucket := range l.buckets {
		if oldestKey == "" || bucket.lastSeen.Before(oldest) {
			oldestKey, oldest = key, bucket.lastSeen
		}
	}
	if oldestKey != "" {
		delete(l.buckets, oldestKey)
	}
}

type rateKey func(*http.Request) string

func limitRequests(limiter *rateLimiter, logger *slog.Logger, scope string, policy ratePolicy, key rateKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := limiter.allow(scope+":"+key(r), policy)
			if allowed {
				next.ServeHTTP(w, r)
				return
			}
			seconds := max(1, int(math.Ceil(retryAfter.Seconds())))
			logger.Warn("request rate limited", "scope", scope, "retry_after_seconds", seconds)
			rateLimitResponse(w, r, seconds)
		})
	}
}

func limitAuthRequests(limiter *rateLimiter, logger *slog.Logger, scope string, ipPolicy, emailPolicy ratePolicy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
			if err := r.ParseForm(); err != nil {
				http.Error(w, frontend.Translate(requestLanguage(r), "Форма слишком большая или некорректная."), http.StatusBadRequest)
				return
			}
			allowed, retryAfter := limiter.allow(scope+":ip:"+clientIP(r), ipPolicy)
			if allowed {
				email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
				digest := sha256.Sum256([]byte(email))
				allowed, retryAfter = limiter.allow(fmt.Sprintf("%s:email:%x", scope, digest), emailPolicy)
			}
			if allowed {
				next.ServeHTTP(w, r)
				return
			}
			seconds := max(1, int(math.Ceil(retryAfter.Seconds())))
			logger.Warn("request rate limited", "scope", scope, "retry_after_seconds", seconds)
			rateLimitResponse(w, r, seconds)
		})
	}
}

func rateLimitResponse(w http.ResponseWriter, r *http.Request, seconds int) {
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	w.Header().Set("Cache-Control", "no-store")
	language := requestLanguage(r)
	w.Header().Set("Content-Language", language)
	http.Error(w, frontend.Translate(language, "Слишком много попыток. Подождите немного и попробуйте снова."), http.StatusTooManyRequests)
}

func requestLanguage(r *http.Request) string {
	if cookie, err := r.Cookie("ui_language"); err == nil && (cookie.Value == "en" || cookie.Value == "az") {
		return cookie.Value
	}
	return "ru"
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if parsed := net.ParseIP(r.RemoteAddr); parsed != nil {
		return parsed.String()
	}
	return "unknown"
}

func ipKey(r *http.Request) string {
	return clientIP(r)
}

func businessIPKey(r *http.Request) string {
	return fmt.Sprintf("%s:%s", chi.URLParam(r, "slug"), clientIP(r))
}

func businessKey(r *http.Request) string {
	return chi.URLParam(r, "slug")
}
