// Package metrics exposes Prometheus metrics for the control plane. Metrics
// contain only operational counts — never tokens, keys, or user data.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ConnectionsTotal counts connection attempts by result.
	ConnectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aevora_connections_total",
		Help: "Connection attempts by result (created, no_gateway, error).",
	}, []string{"result"})

	// AuthTotal counts authentication attempts by result.
	AuthTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aevora_auth_total",
		Help: "Authentication attempts by result (success, failure).",
	}, []string{"result"})

	// gatewaysByStatus is the fleet size by status.
	gatewaysByStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aevora_gateways",
		Help: "Number of gateways by status.",
	}, []string{"status"})

	// activeSessions is the number of active connection leases.
	activeSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aevora_active_sessions",
		Help: "Number of active VPN sessions (leases).",
	})
)

// Handler returns the Prometheus scrape handler.
func Handler() http.Handler { return promhttp.Handler() }

// ConnCreated records a successful connection.
func ConnCreated() { ConnectionsTotal.WithLabelValues("created").Inc() }

// ConnFailed records a failed connection with a coarse reason label.
func ConnFailed(reason string) { ConnectionsTotal.WithLabelValues(reason).Inc() }

// Auth records an auth attempt result ("success" or "failure").
func Auth(result string) { AuthTotal.WithLabelValues(result).Inc() }

// UpdateFleet refreshes the fleet gauges from a periodic snapshot.
func UpdateFleet(gatewaysByStatusCount map[string]int, active int) {
	gatewaysByStatus.Reset()
	for status, n := range gatewaysByStatusCount {
		gatewaysByStatus.WithLabelValues(status).Set(float64(n))
	}
	activeSessions.Set(float64(active))
}
