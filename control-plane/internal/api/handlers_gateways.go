package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/model"
	"github.com/aevora/control-plane/internal/store"
)

// handleGatewayRegister lets an agent self-register a gateway. Authenticated by
// the shared enrollment secret; returns a one-time node token for heartbeats.
func (s *Server) handleGatewayRegister(w http.ResponseWriter, r *http.Request) {
	if s.cfg.EnrollmentSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "gateway registration is disabled")
		return
	}
	if !constEq(bearerToken(r), s.cfg.EnrollmentSecret) {
		writeError(w, http.StatusUnauthorized, "invalid enrollment secret")
		return
	}

	var reg model.GatewayRegistration
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&reg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if msg, ok := validateRegistration(reg); !ok {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if reg.EndpointPort == 0 {
		reg.EndpointPort = 51820
	}

	gw, token, err := s.store.RegisterGateway(r.Context(), reg)
	if err != nil {
		s.log.Error("register gateway", "err", err, "name", reg.Name)
		writeError(w, http.StatusInternalServerError, "could not register gateway")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         gw.ID,
		"name":       gw.Name,
		"status":     gw.Status,
		"node_token": token, // shown once; the server stores only its hash
	})
}

func validateRegistration(r model.GatewayRegistration) (string, bool) {
	switch {
	case r.Name == "":
		return "name is required", false
	case r.CountryCode == "":
		return "country_code is required", false
	case r.CountryName == "":
		return "country_name is required", false
	case r.EndpointHost == "":
		return "endpoint_host is required", false
	case r.PublicKey == "":
		return "public_key is required", false
	case r.WGSubnetV4 == "":
		return "wg_subnet_v4 is required", false
	case r.Capacity <= 0:
		return "capacity must be > 0", false
	}
	return "", true
}

// handleGatewayHeartbeat records live metrics and marks the gateway healthy.
// Authenticated by the per-node token.
func (s *Server) handleGatewayHeartbeat(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing node token")
		return
	}
	var m model.GatewayMetrics
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	gw, err := s.store.Heartbeat(r.Context(), auth.HashToken(token), m)
	if errors.Is(err, store.ErrInvalidToken) {
		writeError(w, http.StatusUnauthorized, "invalid node token")
		return
	}
	if err != nil {
		s.log.Error("gateway heartbeat", "err", err)
		writeError(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"gateway_id": gw.ID, "status": gw.Status})
}

// handleGatewayDeregister marks a gateway disabled on clean agent shutdown.
func (s *Server) handleGatewayDeregister(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing node token")
		return
	}
	err := s.store.DeregisterGateway(r.Context(), auth.HashToken(token))
	if errors.Is(err, store.ErrInvalidToken) {
		writeError(w, http.StatusUnauthorized, "invalid node token")
		return
	}
	if err != nil {
		s.log.Error("gateway deregister", "err", err)
		writeError(w, http.StatusInternalServerError, "deregister failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// gatewayDTO is the admin wire shape of a gateway. It exposes an explicit
// online flag derived from the heartbeat TTL, never any secret material.
type gatewayDTO struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Country         string   `json:"country"`
	CountryCode     string   `json:"country_code"`
	City            string   `json:"city,omitempty"`
	Region          string   `json:"region,omitempty"`
	Endpoint        string   `json:"endpoint"`
	PublicKey       string   `json:"public_key"`
	Status          string   `json:"status"`
	Online          bool     `json:"online"`
	Capacity        int      `json:"capacity"`
	ActivePeers     int      `json:"active_peers"`
	CPUPct          float64  `json:"cpu_pct"`
	RxBps           int64    `json:"rx_bps"`
	TxBps           int64    `json:"tx_bps"`
	BandwidthMbps   int      `json:"bandwidth_mbps,omitempty"`
	Latitude        *float64 `json:"latitude,omitempty"`
	Longitude       *float64 `json:"longitude,omitempty"`
	LastHeartbeatAt *string  `json:"last_heartbeat_at,omitempty"`
}

// handleGatewayList returns the full fleet for the control-plane admin view.
// Guarded by the admin token; disabled (404) when no admin token is configured.
func (s *Server) handleGatewayList(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AdminToken == "" {
		http.NotFound(w, r)
		return
	}
	if !constEq(bearerToken(r), s.cfg.AdminToken) {
		writeError(w, http.StatusUnauthorized, "invalid admin token")
		return
	}

	gws, err := s.store.ListGateways(r.Context())
	if err != nil {
		s.log.Error("list gateways", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list gateways")
		return
	}

	now := time.Now()
	out := make([]gatewayDTO, 0, len(gws))
	for _, g := range gws {
		out = append(out, toGatewayDTO(g, now, s.cfg.HeartbeatTTL))
	}
	writeJSON(w, http.StatusOK, map[string]any{"gateways": out})
}

func toGatewayDTO(g model.Gateway, now time.Time, ttl time.Duration) gatewayDTO {
	var hb *string
	if g.LastHeartbeatAt != nil {
		s := g.LastHeartbeatAt.UTC().Format(time.RFC3339)
		hb = &s
	}
	return gatewayDTO{
		ID:              g.ID,
		Name:            g.Name,
		Country:         g.CountryName,
		CountryCode:     g.LocationCode,
		City:            g.City,
		Region:          g.Region,
		Endpoint:        endpoint(g.EndpointHost, g.EndpointPort),
		PublicKey:       g.PublicKey,
		Status:          string(g.Status),
		Online:          g.Online(now, ttl),
		Capacity:        g.Capacity,
		ActivePeers:     g.ActivePeers,
		CPUPct:          g.CPUPct,
		RxBps:           g.RxBps,
		TxBps:           g.TxBps,
		BandwidthMbps:   g.BandwidthMbps,
		Latitude:        g.Latitude,
		Longitude:       g.Longitude,
		LastHeartbeatAt: hb,
	}
}

func endpoint(host string, port int) string {
	if port == 0 {
		port = 51820
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
