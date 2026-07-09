package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/muradalishov/booking-app/internal/config"
	"github.com/muradalishov/booking-app/internal/db"
	httpserver "github.com/muradalishov/booking-app/internal/http"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Apply schema migrations before serving traffic.
	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	logger.Info("migrations applied")

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("database connected")

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.NewRouter(logger, pool),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start the server in a goroutine so we can wait for a shutdown signal.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	signalCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		return err
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}
