// Package config loads control-plane settings from the environment.
package config

import (
	"os"
	"strconv"
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
	ShutdownGrace  time.Duration
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
		ShutdownGrace:    10 * time.Second,
	}
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
