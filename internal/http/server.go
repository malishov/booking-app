// Package http wires together the HTTP router, middleware, and handlers.
package http

import (
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/muradalishov/booking-app/internal/http/handlers"
	"github.com/muradalishov/booking-app/internal/store"
	"github.com/muradalishov/booking-app/web"
)

// NewRouter wires up all HTTP routes and returns the root handler.
func NewRouter(logger *slog.Logger, pool *pgxpool.Pool, tmpl *template.Template) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	queries := store.New(pool)
	authH := handlers.NewAuth(queries, tmpl, logger)

	r.Method(http.MethodGet, "/health", handlers.NewHealth(pool))

	r.Get("/register", authH.RegisterForm)
	r.Post("/register", authH.Register)
	r.Get("/login", authH.LoginForm)
	r.Post("/login", authH.Login)

	r.Handle("/static/*", http.FileServer(http.FS(web.StaticFS)))

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
