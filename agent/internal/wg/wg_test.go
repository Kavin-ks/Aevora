package wg

import (
	"strings"
	"testing"

	"github.com/aevora/agent/internal/reconcile"
)

// A realistic `wg show <iface> dump`: first line is the interface, then peers.
const sampleDump = "PRIVKEY=\tPUBKEY=\t51820\toff\n" +
	"CLIENT1PUB=\tPSK1=\t203.0.113.9:12345\t10.7.0.2/32,fd07::2/128\t1699999999\t1024\t2048\t25\n" +
	"CLIENT2PUB=\t(none)\t(none)\t10.7.0.3/32\t0\t0\t0\toff\n"

func TestParseDump(t *testing.T) {
	peers := parseDump(sampleDump)
	if len(peers) != 2 {
		t.Fatalf("got %d peers, want 2", len(peers))
	}
	if peers[0].PublicKey != "CLIENT1PUB=" {
		t.Errorf("peer0 key = %q", peers[0].PublicKey)
	}
	if len(peers[0].AllowedIPs) != 2 || peers[0].AllowedIPs[1] != "fd07::2/128" {
		t.Errorf("peer0 allowed = %v", peers[0].AllowedIPs)
	}
	if len(peers[1].AllowedIPs) != 1 || peers[1].AllowedIPs[0] != "10.7.0.3/32" {
		t.Errorf("peer1 allowed = %v", peers[1].AllowedIPs)
	}
}

func TestParseDump_NoPeers(t *testing.T) {
	if peers := parseDump("PRIVKEY=\tPUBKEY=\t51820\toff\n"); len(peers) != 0 {
		t.Fatalf("expected no peers, got %d", len(peers))
	}
}

// Runner.Apply should call the injected runner with the right argument order.
func TestRunner_Apply(t *testing.T) {
	var calls []string
	r := &Runner{iface: "wg0", run: func(args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		return "", nil
	}}
	plan := reconcile.Plan{
		Add:    []reconcile.Peer{{PublicKey: "NEW=", AllowedIPs: []string{"10.7.0.5/32", "fd07::5/128"}}},
		Remove: []string{"OLD="},
	}
	if err := r.Apply(plan); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := []string{
		"set wg0 peer NEW= allowed-ips 10.7.0.5/32,fd07::5/128",
		"set wg0 peer OLD= remove",
	}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls =\n%v\nwant\n%v", calls, want)
	}
}
