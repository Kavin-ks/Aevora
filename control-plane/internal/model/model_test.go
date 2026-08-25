package model

import (
	"testing"
	"time"
)

func TestGateway_Online(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-10 * time.Second)
	stale := now.Add(-2 * time.Minute)
	ttl := 30 * time.Second

	cases := []struct {
		name string
		g    Gateway
		want bool
	}{
		{"healthy+fresh", Gateway{Status: GatewayHealthy, LastHeartbeatAt: &fresh}, true},
		{"healthy+stale", Gateway{Status: GatewayHealthy, LastHeartbeatAt: &stale}, false},
		{"healthy+never", Gateway{Status: GatewayHealthy, LastHeartbeatAt: nil}, false},
		{"unhealthy+fresh", Gateway{Status: GatewayUnhealthy, LastHeartbeatAt: &fresh}, false},
		{"pending+fresh", Gateway{Status: GatewayPending, LastHeartbeatAt: &fresh}, false},
	}
	for _, c := range cases {
		if got := c.g.Online(now, ttl); got != c.want {
			t.Errorf("%s: Online = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLocation_Available(t *testing.T) {
	if !(Location{HealthyCount: 1}).Available() {
		t.Error("location with a healthy gateway should be available")
	}
	if (Location{HealthyCount: 0}).Available() {
		t.Error("location with no healthy gateway should be unavailable")
	}
}
