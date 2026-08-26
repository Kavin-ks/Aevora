package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/metrics"
	"github.com/aevora/control-plane/internal/model"
	"github.com/aevora/control-plane/internal/store"
)

// connectRequest is the body of POST /v1/connections.
type connectRequest struct {
	CountryCode string `json:"country_code"`
	DeviceID    string `json:"device_id"`
}

// connectionResponse is everything the client needs to bring up its tunnel.
type connectionResponse struct {
	ConnectionID string   `json:"connection_id"`
	Server       serverIn `json:"server"`
	AssignedIP   string   `json:"assigned_ip"`
	AssignedIPv6 *string  `json:"assigned_ip6,omitempty"`
	DNS          []string `json:"dns"`
	AllowedIPs   []string `json:"allowed_ips"`
	Keepalive    int      `json:"persistent_keepalive"`
	ExpiresAt    string   `json:"expires_at"`
	// ProbeAddr is the gateway's in-tunnel address:port for the client's latency
	// probe (a TCP round-trip through the tunnel). Empty if it can't be derived.
	ProbeAddr string `json:"probe_addr,omitempty"`
}

// gatewayProbeAddr derives the gateway's in-tunnel address from a client's
// assigned /24 address (the gateway is the .1 of the same subnet) and the probe
// port. Returns "" if the assigned IP can't be parsed as IPv4.
func gatewayProbeAddr(assignedIP string, port int) string {
	host := assignedIP
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return ""
	}
	gw := net.IPv4(ip[0], ip[1], ip[2], 1)
	return net.JoinHostPort(gw.String(), strconv.Itoa(port))
}

type serverIn struct {
	Name      string `json:"name"`
	Country   string `json:"country"`
	City      string `json:"city,omitempty"`
	Endpoint  string `json:"endpoint"`
	PublicKey string `json:"public_key"`
}

// handleConnectionCreate selects a gateway, leases an address, and returns the
// tunnel config. The peer is programmed on the gateway by the node agent.
func (s *Server) handleConnectionCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.authUser(w, r)
	if !ok {
		return
	}
	var req connectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.CountryCode == "" || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "country_code and device_id are required")
		return
	}

	conn, err := s.store.CreateConnection(r.Context(), userID, req.DeviceID, req.CountryCode, s.cfg.LeaseTTL, s.cfg.HeartbeatTTL)
	switch {
	case errors.Is(err, store.ErrDeviceNotFound):
		writeError(w, http.StatusNotFound, "device not found or revoked")
		return
	case errors.Is(err, store.ErrNoGatewayAvailable), errors.Is(err, store.ErrGatewayFull):
		metrics.ConnFailed("no_gateway")
		msg := "no healthy gateway available in that location"
		if errors.Is(err, store.ErrGatewayFull) {
			msg = "selected gateway is at capacity, try again"
		}
		writeError(w, http.StatusServiceUnavailable, msg)
		return
	case err != nil:
		metrics.ConnFailed("error")
		s.log.Error("create connection", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create connection")
		return
	}
	metrics.ConnCreated()

	writeJSON(w, http.StatusCreated, connectionResponse{
		ConnectionID: conn.ID,
		Server: serverIn{
			Name:      conn.GatewayName,
			Country:   conn.Country,
			City:      conn.City,
			Endpoint:  conn.Endpoint,
			PublicKey: conn.GatewayPublicKey,
		},
		AssignedIP:   conn.AssignedIPv4,
		AssignedIPv6: conn.AssignedIPv6,
		DNS:          s.cfg.ClientDNS,
		AllowedIPs:   []string{"0.0.0.0/0", "::/0"},
		Keepalive:    25,
		ExpiresAt:    conn.ExpiresAt.UTC().Format(time.RFC3339),
		ProbeAddr:    gatewayProbeAddr(conn.AssignedIPv4, s.cfg.ProbePort),
	})
}

// handleConnectionDelete releases a connection (disconnect / switch country).
func (s *Server) handleConnectionDelete(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.authUser(w, r)
	if !ok {
		return
	}
	err := s.store.ReleaseConnection(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}
	if err != nil {
		s.log.Error("release connection", "err", err)
		writeError(w, http.StatusInternalServerError, "could not release connection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// statsRequest is the body of POST /v1/connections/{id}/stats. The samples are
// advisory; the call doubles as the lease keep-alive.
type statsRequest struct {
	RxBps     int64 `json:"rx_bps"`
	TxBps     int64 `json:"tx_bps"`
	LatencyMs int   `json:"latency_ms"`
}

// handleConnectionStats renews the lease (keep-alive) and accepts client stats.
func (s *Server) handleConnectionStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.authUser(w, r)
	if !ok {
		return
	}
	var req statsRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req)

	expiresAt, err := s.store.RenewConnection(r.Context(), userID, r.PathValue("id"), s.cfg.LeaseTTL)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}
	if err != nil {
		s.log.Error("renew connection", "err", err)
		writeError(w, http.StatusInternalServerError, "could not renew connection")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"expires_at": expiresAt.UTC().Format(time.RFC3339)})
}

// handleGatewayPeers returns the desired peer set for the calling gateway. The
// node agent (authenticated by its node token) reconciles the interface to it.
func (s *Server) handleGatewayPeers(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing node token")
		return
	}
	peers, err := s.store.GatewayPeers(r.Context(), auth.HashToken(token))
	if errors.Is(err, store.ErrInvalidToken) {
		writeError(w, http.StatusUnauthorized, "invalid node token")
		return
	}
	if err != nil {
		s.log.Error("gateway peers", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list peers")
		return
	}
	if peers == nil {
		peers = []model.Peer{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": peers})
}
