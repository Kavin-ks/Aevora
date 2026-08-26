// Package api exposes the control-plane HTTP/JSON surface.
package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/metrics"
	"github.com/aevora/control-plane/internal/model"
	"github.com/aevora/control-plane/internal/ratelimit"
)

// Store is the slice of persistence the API needs. A narrow interface keeps
// handlers testable with a fake (see handlers_test.go).
type Store interface {
	ListLocations(ctx context.Context) ([]model.Location, error)

	// Gateway fleet (Phase 1c).
	RegisterGateway(ctx context.Context, r model.GatewayRegistration) (model.Gateway, string, error)
	Heartbeat(ctx context.Context, tokenHash string, m model.GatewayMetrics) (model.Gateway, error)
	DeregisterGateway(ctx context.Context, tokenHash string) error
	ListGateways(ctx context.Context) ([]model.Gateway, error)

	// Identity (Phase 1b).
	Enroll(ctx context.Context, r model.EnrollRequest, refreshTTL time.Duration) (model.User, model.Device, string, error)
	CreateDevice(ctx context.Context, userID string, r model.DeviceRegistration, refreshTTL time.Duration) (model.Device, string, error)
	ListDevices(ctx context.Context, userID string) ([]model.Device, error)
	RevokeDevice(ctx context.Context, userID, deviceID string) error
	RefreshAccess(ctx context.Context, refreshPlain string) (userID, deviceID string, err error)
	CreateInvite(ctx context.Context, code, note string, expiresAt *time.Time) error
	SetPassword(ctx context.Context, userID, passwordHash string) error
	GetCredentialsByEmail(ctx context.Context, email string) (userID, passwordHash, status string, err error)

	// Connections (Phase 1d).
	CreateConnection(ctx context.Context, userID, deviceID, country string, leaseTTL, heartbeatTTL time.Duration) (model.Connection, error)
	ReleaseConnection(ctx context.Context, userID, leaseID string) error
	RenewConnection(ctx context.Context, userID, leaseID string, leaseTTL time.Duration) (time.Time, error)
	GatewayPeers(ctx context.Context, tokenHash string) ([]model.Peer, error)
}

// ServerConfig carries the auth secrets and timing the handlers need.
type ServerConfig struct {
	EnrollmentSecret string        // empty => gateway registration disabled
	AdminToken       string        // empty => admin views disabled
	HeartbeatTTL     time.Duration // used to compute online/offline in listings
	JWTSecret        string        // empty => user auth (enroll/refresh/devices) disabled
	AccessTTL        time.Duration // lifetime of an access JWT
	RefreshTTL       time.Duration // lifetime of a refresh token
	LeaseTTL         time.Duration // lifetime of a connection lease before renewal
	ClientDNS        []string      // resolver(s) pushed to connected clients
	AuthRatePerMin   int           // per-IP request/min on auth endpoints (0 => default 10)
	AuthBurst        int           // per-IP burst on auth endpoints (0 => default 5)
}

// Server wires handlers to their dependencies.
type Server struct {
	store       Store
	cfg         ServerConfig
	log         *slog.Logger
	authLimiter *ratelimit.Limiter
}

// NewServer constructs a Server.
func NewServer(store Store, cfg ServerConfig, log *slog.Logger) *Server {
	perMin := cfg.AuthRatePerMin
	if perMin <= 0 {
		perMin = 10
	}
	burst := cfg.AuthBurst
	if burst <= 0 {
		burst = 5
	}
	return &Server{store: store, cfg: cfg, log: log, authLimiter: ratelimit.New(perMin, burst)}
}

// Limiter exposes the auth rate limiter so the caller can run periodic cleanup.
func (s *Server) Limiter() *ratelimit.Limiter { return s.authLimiter }

// Handler returns the fully-routed http.Handler with middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /metrics", metrics.Handler()) // isolate on an internal network in prod
	mux.HandleFunc("GET /v1/locations", s.handleLocations)

	// Identity (user-facing + admin invite minting). Auth endpoints are
	// rate-limited per client IP to resist brute force / abuse.
	mux.HandleFunc("POST /v1/enroll", s.rateLimited(s.handleEnroll))
	mux.HandleFunc("POST /v1/auth/login", s.rateLimited(s.handleLogin))
	mux.HandleFunc("POST /v1/auth/refresh", s.rateLimited(s.handleRefresh))
	mux.HandleFunc("POST /v1/auth/password", s.handleSetPassword)
	mux.HandleFunc("GET /v1/devices", s.handleDeviceList)
	mux.HandleFunc("POST /v1/devices", s.handleDeviceCreate)
	mux.HandleFunc("DELETE /v1/devices/{id}", s.handleDeviceRevoke)
	mux.HandleFunc("POST /v1/invites", s.handleInviteCreate)

	// Connections (user-facing).
	mux.HandleFunc("POST /v1/connections", s.handleConnectionCreate)
	mux.HandleFunc("DELETE /v1/connections/{id}", s.handleConnectionDelete)
	mux.HandleFunc("POST /v1/connections/{id}/stats", s.handleConnectionStats)

	// Gateway fleet (agent-facing + admin).
	mux.HandleFunc("POST /v1/gateways/register", s.handleGatewayRegister)
	mux.HandleFunc("POST /v1/gateways/heartbeat", s.handleGatewayHeartbeat)
	mux.HandleFunc("POST /v1/gateways/deregister", s.handleGatewayDeregister)
	mux.HandleFunc("GET /v1/gateways/peers", s.handleGatewayPeers)
	mux.HandleFunc("GET /v1/gateways", s.handleGatewayList)

	return s.recoverer(s.logger(mux))
}

// authUser authenticates the caller from a Bearer access JWT and returns the
// user id. On failure it writes the response and returns ok=false.
func (s *Server) authUser(w http.ResponseWriter, r *http.Request) (userID string, ok bool) {
	if s.cfg.JWTSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "authentication is disabled")
		return "", false
	}
	tok := bearerToken(r)
	if tok == "" {
		writeError(w, http.StatusUnauthorized, "missing access token")
		return "", false
	}
	claims, err := auth.ParseJWT(s.cfg.JWTSecret, tok)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return "", false
	}
	return claims.Subject, true
}

// rateLimited wraps a handler with per-client-IP throttling.
func (s *Server) rateLimited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authLimiter != nil && !s.authLimiter.Allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next(w, r)
	}
}

// clientIP extracts the client's IP from RemoteAddr. Behind a trusted proxy,
// terminate TLS there and forward the real IP (see deployment docs).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// constEq compares two secrets in constant time (avoids timing side-channels).
func constEq(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// logger logs one line per request with method, path, status and duration.
func (s *Server) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Info("request",
			"method", r.Method, "path", r.URL.Path,
			"status", sw.status, "dur", time.Since(start).String())
	})
}

// recoverer turns a panic into a 500 instead of crashing the process.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", "err", rec, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
