// Package selection ranks gateways for a connection request. It is deliberately
// pure (no I/O) so the policy is unit-testable in isolation.
package selection

import "github.com/aevora/control-plane/internal/model"

// Weights control how selection trades off load vs. bandwidth headroom. The
// scoring is intentionally a pure function of a small struct so the algorithm
// can evolve (e.g. adding client-measured latency as another weighted term)
// without any client change.
type Weights struct {
	Load      float64 // weight on spare peer capacity
	Bandwidth float64 // weight on spare uplink bandwidth
}

// DefaultWeights favour spreading load, with bandwidth headroom as a secondary
// factor when the gateway advertises a bandwidth figure.
var DefaultWeights = Weights{Load: 0.7, Bandwidth: 0.3}

// Fitness returns a 0..1 score for a gateway; higher is better. Only meaningful
// for a healthy gateway (callers must filter first). Combines spare peer
// capacity with spare bandwidth (when advertised). Latency/geo tie-breaking is
// layered on by the caller from client-measured probes in a later iteration.
func Fitness(g model.Gateway) float64 { return FitnessWithWeights(g, DefaultWeights) }

// FitnessWithWeights is Fitness with explicit weights.
func FitnessWithWeights(g model.Gateway, w Weights) float64 {
	if g.Capacity <= 0 {
		return 0
	}
	loadHeadroom := clamp01(1 - float64(g.ActivePeers)/float64(g.Capacity))

	// Only factor bandwidth when the gateway advertises a capacity.
	if g.BandwidthMbps > 0 && (w.Bandwidth > 0) {
		usedBps := float64(g.RxBps + g.TxBps)
		capBps := float64(g.BandwidthMbps) * 1_000_000
		bwHeadroom := clamp01(1 - usedBps/capBps)
		total := w.Load + w.Bandwidth
		if total <= 0 {
			return loadHeadroom
		}
		return (w.Load*loadHeadroom + w.Bandwidth*bwHeadroom) / total
	}
	return loadHeadroom
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
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
