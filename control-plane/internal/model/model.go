// Package model holds the domain types shared across the control plane.
package model

import "time"

// Location is a country a user can pick. Availability is derived from how many
// of its gateways are currently healthy.
type Location struct {
	ID           string `json:"-"`
	Code         string `json:"code"`    // e.g. "de"
	CountryName  string `json:"country"` // e.g. "Germany"
	Enabled      bool   `json:"-"`
	ServerCount  int    `json:"servers"` // total gateways in this location
	HealthyCount int    `json:"-"`
}

// Available reports whether at least one gateway is healthy and selectable.
func (l Location) Available() bool { return l.HealthyCount > 0 }

// GatewayStatus is the lifecycle state of an exit node in the registry.
type GatewayStatus string

const (
	GatewayPending   GatewayStatus = "pending"   // registered, no heartbeat yet
	GatewayHealthy   GatewayStatus = "healthy"    // heartbeating within TTL
	GatewayUnhealthy GatewayStatus = "unhealthy"  // heartbeat expired
	GatewayDraining  GatewayStatus = "draining"   // finishing existing sessions
	GatewayDisabled  GatewayStatus = "disabled"   // deregistered / taken out
)

// Gateway is an exit node in a location, with its metadata and live metrics.
// Only the PUBLIC WireGuard key is ever stored here.
type Gateway struct {
	ID            string
	LocationID    string
	LocationCode  string // country code, e.g. "de"
	CountryName   string
	Name          string // unique identity, e.g. "de-fra-1"
	City          string
	Region        string // provider/geographic region, e.g. "eu-central"
	PublicKey     string
	EndpointHost  string
	EndpointPort  int
	Capacity      int     // max concurrent peers
	BandwidthMbps int     // uplink hint for future bandwidth-aware selection
	Latitude      *float64
	Longitude     *float64

	Status          GatewayStatus
	ActivePeers     int
	CPUPct          float64
	RxBps           int64
	TxBps           int64
	LastHeartbeatAt *time.Time
}

// Online reports whether the gateway is healthy and has heartbeated within ttl
// of now. This is the online/offline signal the control plane and selection use.
func (g Gateway) Online(now time.Time, ttl time.Duration) bool {
	if g.Status != GatewayHealthy || g.LastHeartbeatAt == nil {
		return false
	}
	return now.Sub(*g.LastHeartbeatAt) <= ttl
}

// GatewayRegistration is the metadata an agent supplies to self-register. A new
// country is created implicitly from CountryCode/CountryName, so adding a
// location is just bringing up a gateway there.
type GatewayRegistration struct {
	Name          string   `json:"name"`
	CountryCode   string   `json:"country_code"`
	CountryName   string   `json:"country_name"`
	City          string   `json:"city"`
	Region        string   `json:"region"`
	EndpointHost  string   `json:"endpoint_host"`
	EndpointPort  int      `json:"endpoint_port"`
	PublicKey     string   `json:"public_key"`
	WGSubnetV4    string   `json:"wg_subnet_v4"`
	WGSubnetV6    string   `json:"wg_subnet_v6"`
	Capacity      int      `json:"capacity"`
	BandwidthMbps int      `json:"bandwidth_mbps"`
	Latitude      *float64 `json:"latitude"`
	Longitude     *float64 `json:"longitude"`
}

// GatewayMetrics is the live load an agent reports on each heartbeat.
type GatewayMetrics struct {
	ActivePeers int     `json:"active_peers"`
	CPUPct      float64 `json:"cpu_pct"`
	RxBps       int64   `json:"rx_bps"`
	TxBps       int64   `json:"tx_bps"`
}
