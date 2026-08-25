// Command aevora-controld is the Aevora control-plane API server.
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

	"github.com/aevora/control-plane/internal/api"
	"github.com/aevora/control-plane/internal/config"
	"github.com/aevora/control-plane/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.FromEnv()

	// Connect + migrate with a bounded startup timeout.
	startCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.New(startCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := st.Migrate(startCtx); err != nil {
		return err
	}
	if cfg.DevSeed {
		if err := st.SeedDev(startCtx); err != nil {
			return err
		}
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.NewServer(st, log).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Serve until a signal, then shut down gracefully.
	errCh := make(chan error, 1)
	go func() {
		log.Info("control plane listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
		log.Info("shutting down")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
		defer shutCancel()
		return srv.Shutdown(shutCtx)
	}
}
