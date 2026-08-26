// Package config loads control-plane settings from the environment.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings. Everything comes from the environment so
// the same binary runs identically in docker-compose and in production.
type Config struct {
	ListenAddr       string // AEVORA_LISTEN_ADDR   (default :8080)
	DatabaseURL      string // AEVORA_DB_URL         (required)
	EnrollmentSecret string // AEVORA_ENROLLMENT_SECRET — secret agents present to self-register (empty disables registration)
	AdminToken       string // AEVORA_ADMIN_TOKEN    — bearer for the control-plane admin views (empty disables them)
	JWTSecret        string // AEVORA_JWT_SECRET     — signs user access tokens
	DevSeed          bool   // AEVORA_DEV_SEED=1     — insert idempotent dev data on boot

	// HeartbeatTTL: a gateway is considered offline once its last heartbeat is
	// older than this. ReaperInterval: how often stale gateways are swept.
	HeartbeatTTL   time.Duration // AEVORA_HEARTBEAT_TTL   (default 30s)
	ReaperInterval time.Duration // AEVORA_REAPER_INTERVAL (default 10s)

	// AccessTTL: lifetime of a user access JWT. RefreshTTL: lifetime of a
	// refresh token used to mint new access tokens without re-enrolling.
	AccessTTL  time.Duration // AEVORA_ACCESS_TTL  (default 15m)
	RefreshTTL time.Duration // AEVORA_REFRESH_TTL (default 720h = 30d)

	// LeaseTTL: how long a connection lease is valid before it must be renewed
	// (via the stats keep-alive); an unrenewed lease is reaped and its peer
	// removed. ClientDNS: resolver(s) pushed to connected clients.
	LeaseTTL  time.Duration // AEVORA_LEASE_TTL  (default 5m)
	ClientDNS []string      // AEVORA_CLIENT_DNS (default 9.9.9.9,149.112.112.112)

	// Per-client-IP throttling on auth endpoints (login/enroll/refresh).
	AuthRatePerMin int  // AEVORA_AUTH_RATE_PER_MIN (default 10)
	AuthBurst      int  // AEVORA_AUTH_BURST        (default 5)
	TrustProxy     bool // AEVORA_TRUST_PROXY       — behind a reverse proxy, honor X-Forwarded-For

	ShutdownGrace time.Duration
}

// FromEnv builds a Config, applying sane defaults for local development.
func FromEnv() Config {
	return Config{
		ListenAddr:       getenv("AEVORA_LISTEN_ADDR", ":8080"),
		DatabaseURL:      getenv("AEVORA_DB_URL", "postgres://aevora:aevora@localhost:5432/aevora?sslmode=disable"),
		EnrollmentSecret: getenv("AEVORA_ENROLLMENT_SECRET", ""),
		AdminToken:       getenv("AEVORA_ADMIN_TOKEN", ""),
		JWTSecret:        getenv("AEVORA_JWT_SECRET", ""),
		DevSeed:          getbool("AEVORA_DEV_SEED", false),
		HeartbeatTTL:     getdur("AEVORA_HEARTBEAT_TTL", 30*time.Second),
		ReaperInterval:   getdur("AEVORA_REAPER_INTERVAL", 10*time.Second),
		AccessTTL:        getdur("AEVORA_ACCESS_TTL", 15*time.Minute),
		RefreshTTL:       getdur("AEVORA_REFRESH_TTL", 720*time.Hour),
		LeaseTTL:         getdur("AEVORA_LEASE_TTL", 5*time.Minute),
		ClientDNS:        getcsv("AEVORA_CLIENT_DNS", []string{"9.9.9.9", "149.112.112.112"}),
		AuthRatePerMin:   getint("AEVORA_AUTH_RATE_PER_MIN", 10),
		AuthBurst:        getint("AEVORA_AUTH_BURST", 5),
		TrustProxy:       getbool("AEVORA_TRUST_PROXY", false),
		ShutdownGrace:    10 * time.Second,
	}
}

func getint(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getcsv(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func getdur(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getbool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
