// Package cp is the agent's client for the Aevora control-plane API.
package cp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aevora/agent/internal/reconcile"
)

// Client talks to the control plane over HTTPS/JSON.
type Client struct {
	base string
	http *http.Client
}

// New returns a Client for the given control-plane base URL.
func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// Registration is the gateway metadata sent at self-registration.
type Registration struct {
	Name          string   `json:"name"`
	CountryCode   string   `json:"country_code"`
	CountryName   string   `json:"country_name"`
	City          string   `json:"city,omitempty"`
	Region        string   `json:"region,omitempty"`
	EndpointHost  string   `json:"endpoint_host"`
	EndpointPort  int      `json:"endpoint_port,omitempty"`
	PublicKey     string   `json:"public_key"`
	WGSubnetV4    string   `json:"wg_subnet_v4"`
	WGSubnetV6    string   `json:"wg_subnet_v6,omitempty"`
	Capacity      int      `json:"capacity"`
	BandwidthMbps int      `json:"bandwidth_mbps,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
}

// Metrics is the live load reported on each heartbeat.
type Metrics struct {
	ActivePeers int     `json:"active_peers"`
	CPUPct      float64 `json:"cpu_pct"`
	RxBps       int64   `json:"rx_bps"`
	TxBps       int64   `json:"tx_bps"`
}

// Register self-registers the gateway and returns the one-time node token.
func (c *Client) Register(ctx context.Context, enrollmentSecret string, reg Registration) (string, error) {
	var out struct {
		NodeToken string `json:"node_token"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/gateways/register", enrollmentSecret, reg, &out); err != nil {
		return "", err
	}
	if out.NodeToken == "" {
		return "", fmt.Errorf("register: empty node token in response")
	}
	return out.NodeToken, nil
}

// Heartbeat reports load and marks the gateway healthy.
func (c *Client) Heartbeat(ctx context.Context, nodeToken string, m Metrics) error {
	return c.do(ctx, http.MethodPost, "/v1/gateways/heartbeat", nodeToken, m, nil)
}

// Peers fetches the desired peer set for this gateway.
func (c *Client) Peers(ctx context.Context, nodeToken string) ([]reconcile.Peer, error) {
	var out struct {
		Peers []struct {
			PublicKey  string   `json:"public_key"`
			AllowedIPs []string `json:"allowed_ips"`
		} `json:"peers"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/gateways/peers", nodeToken, nil, &out); err != nil {
		return nil, err
	}
	peers := make([]reconcile.Peer, 0, len(out.Peers))
	for _, p := range out.Peers {
		peers = append(peers, reconcile.Peer{PublicKey: p.PublicKey, AllowedIPs: p.AllowedIPs})
	}
	return peers, nil
}

// Deregister marks the gateway disabled (clean shutdown).
func (c *Client) Deregister(ctx context.Context, nodeToken string) error {
	return c.do(ctx, http.MethodPost, "/v1/gateways/deregister", nodeToken, nil, nil)
}

// do performs a JSON request with a Bearer token, decoding into out if non-nil.
func (c *Client) do(ctx context.Context, method, path, bearer string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
