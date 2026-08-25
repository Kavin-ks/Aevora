package store

import (
	"encoding/binary"
	"net"
	"testing"
)

func u32(ip string) uint32 { return binary.BigEndian.Uint32(net.ParseIP(ip).To4()) }

func TestAllocateHost_FirstFree(t *testing.T) {
	v6cidr := "fd07:0007:1::/64"
	v4, v6, err := allocateHost("10.7.50.0/24", &v6cidr, map[uint32]bool{})
	if err != nil {
		t.Fatalf("alloc: %v", err)
	}
	if v4 != "10.7.50.2" { // .0 network, .1 gateway are skipped
		t.Fatalf("v4 = %s, want 10.7.50.2", v4)
	}
	if v6 == nil || *v6 != "fd07:7:1::2" { // Go canonicalizes (strips leading zeros)
		t.Fatalf("v6 = %v, want fd07:7:1::2", derefOr(v6))
	}
}

func derefOr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestAllocateHost_SkipsUsed(t *testing.T) {
	used := map[uint32]bool{u32("10.7.50.2"): true, u32("10.7.50.3"): true}
	v4, _, err := allocateHost("10.7.50.0/24", nil, used)
	if err != nil {
		t.Fatalf("alloc: %v", err)
	}
	if v4 != "10.7.50.4" {
		t.Fatalf("v4 = %s, want 10.7.50.4", v4)
	}
}

func TestAllocateHost_NoV6WhenSubnetNil(t *testing.T) {
	_, v6, err := allocateHost("10.7.50.0/24", nil, map[uint32]bool{})
	if err != nil {
		t.Fatalf("alloc: %v", err)
	}
	if v6 != nil {
		t.Fatalf("v6 = %v, want nil", v6)
	}
}

func TestAllocateHost_Exhausted(t *testing.T) {
	// Fill every usable host in a /24 (.2 .. .254).
	used := map[uint32]bool{}
	base := u32("10.7.50.0")
	for h := uint32(2); h <= 254; h++ {
		used[base+h] = true
	}
	if _, _, err := allocateHost("10.7.50.0/24", nil, used); err != ErrGatewayFull {
		t.Fatalf("err = %v, want ErrGatewayFull", err)
	}
}
