// Command aevora-agent runs on each gateway. It self-registers with the control
// plane, heartbeats its load, and reconciles the WireGuard interface's peers to
// match the leases the control plane has issued. It never accepts inbound
// connections and stores no client private keys.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aevora/agent/internal/cp"
	"github.com/aevora/agent/internal/reconcile"
	"github.com/aevora/agent/internal/wg"
)

type config struct {
	controlURL       string
	iface            string
	tokenFile        string
	enrollmentSecret string
	syncInterval     time.Duration
	reg              cp.Registration
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := loadConfig()
	client := cp.New(cfg.controlURL)
	iface := wg.New(cfg.iface)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Obtain (or reuse) the node token.
	token, err := ensureRegistered(ctx, log, cfg, client, iface)
	if err != nil {
		return err
	}
	log.Info("agent registered", "interface", cfg.iface, "control", cfg.controlURL)

	ticker := time.NewTicker(cfg.syncInterval)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-stop:
			log.Info("deregistering on shutdown")
			dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = client.Deregister(dctx, token)
			dcancel()
			return nil
		case <-ticker.C:
			if err := syncOnce(ctx, log, client, iface, token); err != nil {
				log.Error("sync", "err", err)
			}
		}
	}
}

// syncOnce heartbeats current load and reconciles peers to the desired set.
func syncOnce(ctx context.Context, log *slog.Logger, client *cp.Client, iface *wg.Runner, token string) error {
	current, err := iface.ListPeers()
	if err != nil {
		return err
	}
	if err := client.Heartbeat(ctx, token, cp.Metrics{ActivePeers: len(current)}); err != nil {
		return err
	}
	desired, err := client.Peers(ctx, token)
	if err != nil {
		return err
	}
	plan := reconcile.Diff(current, desired)
	if plan.Empty() {
		return nil
	}
	if err := iface.Apply(plan); err != nil {
		return err
	}
	log.Info("reconciled peers", "added", len(plan.Add), "removed", len(plan.Remove))
	return nil
}

// ensureRegistered loads a saved node token, or registers to obtain one.
func ensureRegistered(ctx context.Context, log *slog.Logger, cfg config, client *cp.Client, iface *wg.Runner) (string, error) {
	if b, err := os.ReadFile(cfg.tokenFile); err == nil {
		if tok := strings.TrimSpace(string(b)); tok != "" {
			return tok, nil
		}
	}
	// First run: fill the public key from the live interface and register.
	pub, err := iface.PublicKey()
	if err != nil {
		return "", err
	}
	cfg.reg.PublicKey = pub

	log.Info("registering gateway", "name", cfg.reg.Name, "country", cfg.reg.CountryCode)
	token, err := client.Register(ctx, cfg.enrollmentSecret, cfg.reg)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(cfg.tokenFile, []byte(token), 0o600); err != nil {
		log.Warn("could not persist node token; will re-register on restart", "err", err)
	}
	return token, nil
}

func loadConfig() config {
	return config{
		controlURL:       getenv("AEVORA_CONTROL_URL", "http://127.0.0.1:8080"),
		iface:            getenv("AEVORA_WG_INTERFACE", "wg0"),
		tokenFile:        getenv("AEVORA_NODE_TOKEN_FILE", "/etc/aevora/node.token"),
		enrollmentSecret: os.Getenv("AEVORA_ENROLLMENT_SECRET"),
		syncInterval:     getdur("AEVORA_SYNC_INTERVAL", 3*time.Second),
		reg: cp.Registration{
			Name:          os.Getenv("AEVORA_GATEWAY_NAME"),
			CountryCode:   os.Getenv("AEVORA_COUNTRY_CODE"),
			CountryName:   os.Getenv("AEVORA_COUNTRY_NAME"),
			City:          os.Getenv("AEVORA_CITY"),
			Region:        os.Getenv("AEVORA_REGION"),
			EndpointHost:  os.Getenv("AEVORA_ENDPOINT_HOST"),
			EndpointPort:  getint("AEVORA_ENDPOINT_PORT", 51820),
			WGSubnetV4:    os.Getenv("AEVORA_WG_SUBNET_V4"),
			WGSubnetV6:    os.Getenv("AEVORA_WG_SUBNET_V6"),
			Capacity:      getint("AEVORA_CAPACITY", 250),
			BandwidthMbps: getint("AEVORA_BANDWIDTH_MBPS", 0),
		},
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getint(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getdur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
