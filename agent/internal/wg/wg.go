// Package wg drives the WireGuard interface by shelling out to the `wg` tool
// (the same tool the Phase 0 gateway kit installs). The command runner is
// injectable so the dump parser can be tested without a real interface.
package wg

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/aevora/agent/internal/reconcile"
)

// Runner applies and reads WireGuard peer state on one interface.
type Runner struct {
	iface string
	run   func(args ...string) (string, error)
}

// New returns a Runner that executes the real `wg` binary.
func New(iface string) *Runner {
	return &Runner{
		iface: iface,
		run: func(args ...string) (string, error) {
			out, err := exec.Command("wg", args...).CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("wg %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
			return string(out), nil
		},
	}
}

// PublicKey returns the interface's WireGuard public key.
func (r *Runner) PublicKey() (string, error) {
	out, err := r.run("show", r.iface, "public-key")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ListPeers returns the interface's current peers.
func (r *Runner) ListPeers() ([]reconcile.Peer, error) {
	out, err := r.run("show", r.iface, "dump")
	if err != nil {
		return nil, err
	}
	return parseDump(out), nil
}

// SetPeer adds or updates a peer's allowed IPs.
func (r *Runner) SetPeer(p reconcile.Peer) error {
	_, err := r.run("set", r.iface, "peer", p.PublicKey, "allowed-ips", strings.Join(p.AllowedIPs, ","))
	return err
}

// RemovePeer removes a peer by public key.
func (r *Runner) RemovePeer(publicKey string) error {
	_, err := r.run("set", r.iface, "peer", publicKey, "remove")
	return err
}

// Apply executes a reconcile plan: updates/adds first, then removals.
func (r *Runner) Apply(plan reconcile.Plan) error {
	for _, p := range plan.Add {
		if err := r.SetPeer(p); err != nil {
			return err
		}
	}
	for _, pk := range plan.Remove {
		if err := r.RemovePeer(pk); err != nil {
			return err
		}
	}
	return nil
}

// parseDump parses `wg show <iface> dump`. The first line describes the
// interface itself and is skipped; each remaining line is a peer, tab-separated:
//
//	public-key  preshared-key  endpoint  allowed-ips  latest-handshake  rx  tx  keepalive
func parseDump(out string) []reconcile.Peer {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var peers []reconcile.Peer
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // interface line, or blank
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		var ips []string
		if allowed := f[3]; allowed != "" && allowed != "(none)" {
			for _, ip := range strings.Split(allowed, ",") {
				if ip = strings.TrimSpace(ip); ip != "" {
					ips = append(ips, ip)
				}
			}
		}
		peers = append(peers, reconcile.Peer{PublicKey: f[0], AllowedIPs: ips})
	}
	return peers
}
