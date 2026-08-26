// Package probe runs a tiny TCP responder on the gateway's in-tunnel address so
// clients can measure real round-trip latency THROUGH the WireGuard tunnel by
// timing a TCP handshake to it. It binds to the gateway's inside IP, so it is
// only reachable from inside the tunnel, not the public internet.
package probe

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"
)

// InsideIP returns the gateway's in-tunnel address (the .1 of the WireGuard
// IPv4 subnet), e.g. "10.7.1.0/24" -> "10.7.1.1".
func InsideIP(cidr string) (string, error) {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	ip := n.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("not an IPv4 subnet: %s", cidr)
	}
	return net.IPv4(ip[0], ip[1], ip[2], 1).String(), nil
}

// Serve accepts and immediately closes TCP connections on addr until ctx is
// cancelled. The handshake completing is the client's latency measurement. It
// retries the bind because the WireGuard interface may not be up yet at start.
func Serve(ctx context.Context, addr string, log *slog.Logger) {
	var ln net.Listener
	for {
		var err error
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			break
		}
		log.Warn("latency probe: bind failed, retrying", "addr", addr, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
	defer ln.Close()
	go func() { <-ctx.Done(); ln.Close() }()

	log.Info("latency probe listening", "addr", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		_ = conn.Close()
	}
}
