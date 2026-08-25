// Package api exposes the control-plane HTTP/JSON surface.
package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aevora/control-plane/internal/model"
)

// Store is the slice of persistence the API needs. A narrow interface keeps
// handlers testable with a fake (see handlers_test.go).
type Store interface {
	ListLocations(ctx context.Context) ([]model.Location, error)
	RegisterGateway(ctx context.Context, r model.GatewayRegistration) (model.Gateway, string, error)
	Heartbeat(ctx context.Context, tokenHash string, m model.GatewayMetrics) (model.Gateway, error)
	DeregisterGateway(ctx context.Context, tokenHash string) error
	ListGateways(ctx context.Context) ([]model.Gateway, error)
}

// ServerConfig carries the auth secrets and timing the handlers need.
type ServerConfig struct {
	EnrollmentSecret string        // empty => gateway registration disabled
	AdminToken       string        // empty => admin views disabled
	HeartbeatTTL     time.Duration // used to compute online/offline in listings
}

// Server wires handlers to their dependencies.
type Server struct {
	store Store
	cfg   ServerConfig
	log   *slog.Logger
}

// NewServer constructs a Server.
func NewServer(store Store, cfg ServerConfig, log *slog.Logger) *Server {
	return &Server{store: store, cfg: cfg, log: log}
}

// Handler returns the fully-routed http.Handler with middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/locations", s.handleLocations)

	// Gateway fleet (agent-facing + admin).
	mux.HandleFunc("POST /v1/gateways/register", s.handleGatewayRegister)
	mux.HandleFunc("POST /v1/gateways/heartbeat", s.handleGatewayHeartbeat)
	mux.HandleFunc("POST /v1/gateways/deregister", s.handleGatewayDeregister)
	mux.HandleFunc("GET /v1/gateways", s.handleGatewayList)

	return s.recoverer(s.logger(mux))
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
