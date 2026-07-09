package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Pinger is the subset of the database pool the health check depends on.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health reports service liveness, including database connectivity.
type Health struct {
	db Pinger
}

// NewHealth constructs a Health handler.
func NewHealth(db Pinger) *Health {
	return &Health{db: db}
}

// ServeHTTP returns 200 if the database is reachable, 503 otherwise.
func (h *Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := "ok"
	code := http.StatusOK
	if err := h.db.Ping(ctx); err != nil {
		status = "unavailable"
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
}
