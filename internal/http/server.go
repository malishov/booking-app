package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/muradalishov/booking-app/internal/http/handlers"
)

// Pinger is implemented by the database pool used for the health check.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NewRouter wires up all HTTP routes and returns the root handler.
func NewRouter(logger *slog.Logger, db Pinger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Method(http.MethodGet, "/health", handlers.NewHealth(db))

	return r
}

// requestLogger is a small slog-based request logging middleware.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", middleware.GetReqID(r.Context()),
			)
			next.ServeHTTP(w, r)
		})
	}
}
