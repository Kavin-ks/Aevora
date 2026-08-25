// Package model holds the domain types shared across the control plane.
package model

import "time"

// Location is a country a user can pick. Availability is derived from how many
// of its gateways are currently healthy.
type Location struct {
	ID           string `json:"-"`
	Code         string `json:"code"`         // e.g. "de"
	CountryName  string `json:"country"`      // e.g. "Germany"
	Enabled      bool   `json:"-"`
	ServerCount  int    `json:"servers"`      // total gateways in this location
	HealthyCount int    `json:"-"`
}

// Available reports whether at least one gateway is healthy and selectable.
func (l Location) Available() bool { return l.HealthyCount > 0 }

// GatewayStatus is the lifecycle state of an exit node in the registry.
type GatewayStatus string

const (
	GatewayPending   GatewayStatus = "pending"
	GatewayHealthy   GatewayStatus = "healthy"
	GatewayUnhealthy GatewayStatus = "unhealthy"
	GatewayDraining  GatewayStatus = "draining"
	GatewayDisabled  GatewayStatus = "disabled"
)

// Gateway is an exit node in a location, with its live fitness metrics.
type Gateway struct {
	ID              string
	LocationID      string
	Name            string        // "de-fra-1"
	City            string
	PublicKey       string        // gateway WireGuard public key
	EndpointHost    string
	EndpointPort    int
	Capacity        int           // max concurrent peers
	Status          GatewayStatus
	ActivePeers     int
	CPUPct          float64
	RxBps           int64
	TxBps           int64
	LastHeartbeatAt *time.Time
}
