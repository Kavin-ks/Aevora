package probe

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestInsideIP(t *testing.T) {
	got, err := InsideIP("10.7.1.0/24")
	if err != nil || got != "10.7.1.1" {
		t.Fatalf("InsideIP = %q, %v; want 10.7.1.1", got, err)
	}
	if _, err := InsideIP("nonsense"); err == nil {
		t.Fatal("expected error for bad CIDR")
	}
}

func TestServe_AcceptsHandshake(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Bind an ephemeral port on loopback (stand-in for the in-tunnel IP).
	addr := "127.0.0.1:0"
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("pre-bind: %v", err)
	}
	realAddr := ln.Addr().String()
	ln.Close() // free it for Serve

	go Serve(ctx, realAddr, log)

	// Give Serve a moment to bind, then a TCP handshake must succeed.
	var connected bool
	for i := 0; i < 50; i++ {
		if c, err := net.DialTimeout("tcp", realAddr, 200*time.Millisecond); err == nil {
			c.Close()
			connected = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !connected {
		t.Fatal("probe responder did not accept a handshake")
	}
}
