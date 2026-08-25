// Package api exposes the control-plane HTTP/JSON surface.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/aevora/control-plane/internal/model"
)

// LocationLister is the slice of the store the location handler needs. Narrow
// interfaces keep handlers testable with fakes (see handlers_test.go).
type LocationLister interface {
	ListLocations(ctx context.Context) ([]model.Location, error)
}

// Server wires handlers to their dependencies.
type Server struct {
	locations LocationLister
	log       *slog.Logger
}

// NewServer constructs a Server.
func NewServer(locations LocationLister, log *slog.Logger) *Server {
	return &Server{locations: locations, log: log}
}

// Handler returns the fully-routed http.Handler with middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/locations", s.handleLocations)
	return s.recoverer(s.logger(mux))
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
