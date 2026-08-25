// Package reconcile computes the difference between the WireGuard peers the
// gateway currently has and the peers the control plane says it should have.
// It is pure (no I/O) so the policy is fully unit-testable.
package reconcile

import "sort"

// Peer is one WireGuard peer: a public key and its allowed IPs.
type Peer struct {
	PublicKey  string
	AllowedIPs []string
}

// Plan is the set of changes to bring current in line with desired.
type Plan struct {
	Add    []Peer   // peers to add or update (allowed-ips changed)
	Remove []string // public keys to remove
}

// Empty reports whether the plan is a no-op.
func (p Plan) Empty() bool { return len(p.Add) == 0 && len(p.Remove) == 0 }

// Diff returns the plan that transforms current into desired. A peer is added
// when it is new or its allowed-IP set changed; removed when it is no longer
// desired. Output is sorted by public key for determinism.
func Diff(current, desired []Peer) Plan {
	cur := index(current)
	des := index(desired)

	var plan Plan
	for pk, dips := range des {
		if cips, ok := cur[pk]; !ok || !sameSet(cips, dips) {
			plan.Add = append(plan.Add, Peer{PublicKey: pk, AllowedIPs: dips})
		}
	}
	for pk := range cur {
		if _, ok := des[pk]; !ok {
			plan.Remove = append(plan.Remove, pk)
		}
	}

	sort.Slice(plan.Add, func(i, j int) bool { return plan.Add[i].PublicKey < plan.Add[j].PublicKey })
	sort.Strings(plan.Remove)
	return plan
}

func index(peers []Peer) map[string][]string {
	m := make(map[string][]string, len(peers))
	for _, p := range peers {
		m[p.PublicKey] = p.AllowedIPs
	}
	return m
}

// sameSet compares two string slices as sets (order-insensitive).
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, x := range a {
		seen[x]++
	}
	for _, y := range b {
		if seen[y] == 0 {
			return false
		}
		seen[y]--
	}
	return true
}
