package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/config"
	"github.com/commerceops/commerceops/services/api/internal/health"
	"github.com/commerceops/commerceops/services/api/internal/platform/database"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.Handle("/api/v1/health", health.NewHandler(db, cfg.DatabaseTimeout))
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpserver.Middleware(logger, cfg.AllowedOrigins, mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "address", cfg.HTTPAddr, "environment", cfg.Environment)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) { return nil }
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		logger.Info("http server shutting down")
		return server.Shutdown(shutdownCtx)
	}
}
