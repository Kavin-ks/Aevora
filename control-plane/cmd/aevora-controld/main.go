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
	"github.com/aevora/control-plane/internal/metrics"
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

	// Background context for long-running workers; canceled on shutdown.
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	go runReaper(bgCtx, st, cfg.ReaperInterval, cfg.HeartbeatTTL, log)

	apiSrv := api.NewServer(st, api.ServerConfig{
		EnrollmentSecret: cfg.EnrollmentSecret,
		AdminToken:       cfg.AdminToken,
		HeartbeatTTL:     cfg.HeartbeatTTL,
		JWTSecret:        cfg.JWTSecret,
		AccessTTL:        cfg.AccessTTL,
		RefreshTTL:       cfg.RefreshTTL,
		LeaseTTL:         cfg.LeaseTTL,
		ClientDNS:        cfg.ClientDNS,
		AuthRatePerMin:   cfg.AuthRatePerMin,
		AuthBurst:        cfg.AuthBurst,
		TrustProxy:       cfg.TrustProxy,
	}, log)

	// Periodically evict idle rate-limiter entries.
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-bgCtx.Done():
				return
			case <-t.C:
				apiSrv.Limiter().Cleanup()
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           apiSrv.Handler(),
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
		bgCancel()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
		defer shutCancel()
		return srv.Shutdown(shutCtx)
	}
}

// runReaper periodically flips gateways whose heartbeat has expired to
// unhealthy, so a dead node leaves the selection pool without operator action.
func runReaper(ctx context.Context, st *store.Store, interval, ttl time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := st.MarkStaleUnhealthy(ctx, ttl); err != nil {
				if ctx.Err() == nil {
					log.Error("reaper: gateways", "err", err)
				}
			} else if n > 0 {
				log.Info("reaper marked gateways offline", "count", n, "ttl", ttl.String())
			}
			if n, err := st.ExpireStaleLeases(ctx); err != nil {
				if ctx.Err() == nil {
					log.Error("reaper: leases", "err", err)
				}
			} else if n > 0 {
				log.Info("reaper expired stale leases", "count", n)
			}
			if n, err := st.ExpireLeasesOnUnhealthyGateways(ctx); err != nil {
				if ctx.Err() == nil {
					log.Error("reaper: failover", "err", err)
				}
			} else if n > 0 {
				log.Info("reaper expired leases on unhealthy gateways (failover)", "count", n)
			}
			// Refresh fleet metrics from a periodic snapshot.
			if byStatus, active, err := st.FleetSnapshot(ctx); err == nil {
				metrics.UpdateFleet(byStatus, active)
			} else if ctx.Err() == nil {
				log.Error("reaper: metrics snapshot", "err", err)
			}
		}
	}
}
