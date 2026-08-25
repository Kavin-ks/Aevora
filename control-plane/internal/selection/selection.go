// Package selection ranks gateways for a connection request. It is deliberately
// pure (no I/O) so the policy is unit-testable in isolation.
package selection

import "github.com/aevora/control-plane/internal/model"

// Fitness returns a 0..1 score for a gateway; higher is better. It is only
// meaningful for a healthy gateway (callers must filter first). For now it is a
// spare-capacity measure: an empty gateway scores ~1, a full one ~0. Phase 3
// folds in client latency and bandwidth headroom as additional weighted terms.
func Fitness(g model.Gateway) float64 {
	if g.Capacity <= 0 {
		return 0
	}
	util := float64(g.ActivePeers) / float64(g.Capacity)
	if util < 0 {
		util = 0
	}
	if util > 1 {
		util = 1
	}
	return 1 - util
}

// Best returns the highest-fitness healthy gateway from candidates. Gateways
// that are not healthy or are at/over capacity are excluded. ok is false when
// nothing selectable remains.
func Best(candidates []model.Gateway) (best model.Gateway, ok bool) {
	bestScore := -1.0
	for _, g := range candidates {
		if g.Status != model.GatewayHealthy {
			continue
		}
		if g.Capacity > 0 && g.ActivePeers >= g.Capacity {
			continue // no spare slots
		}
		if s := Fitness(g); s > bestScore {
			bestScore, best, ok = s, g, true
		}
	}
	return best, ok
}
