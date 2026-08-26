package sysload

import "testing"

func TestParseProcStat(t *testing.T) {
	// user=100 nice=0 system=50 idle=800 iowait=50 => total=1000, idle=850
	total, idle := parseProcStat("cpu  100 0 50 800 50 0 0 0\ncpu0 1 2 3 4\n")
	if total != 1000 {
		t.Fatalf("total = %d, want 1000", total)
	}
	if idle != 850 {
		t.Fatalf("idle = %d, want 850 (idle+iowait)", idle)
	}
}

func TestMeter_ComputesDeltas(t *testing.T) {
	// Scripted readers: first baseline, then a second reading.
	stats := []string{
		"cpu 100 0 100 800 0 0 0 0",  // total 1000, idle 800
		"cpu 300 0 300 1400 0 0 0 0", // total 2000 (+1000), idle 1400 (+600) => busy 400/1000 = 40%
	}
	nets := []struct{ rx, tx uint64 }{
		{1000, 2000},
		{1000 + 1_250_000, 2000 + 2_500_000}, // over ~1s => bits: 10Mbps rx, 20Mbps tx (approx)
	}
	i := 0
	m := &Meter{
		wanIface:     "eth0",
		readProcStat: func() (string, error) { return stats[min(i, len(stats)-1)], nil },
		readNetBytes: func(string) (uint64, uint64, error) {
			n := nets[min(i, len(nets)-1)]
			return n.rx, n.tx, nil
		},
	}

	first := m.Sample() // baseline, zeros
	if first.CPUPct != 0 || first.RxBps != 0 {
		t.Fatalf("first sample should be zero baseline, got %+v", first)
	}

	i = 1
	// Force a measurable interval for the net rate.
	m.prevNet.at = m.prevNet.at.Add(-1_000_000_000) // pretend 1s elapsed
	s := m.Sample()
	if s.CPUPct < 39 || s.CPUPct > 41 {
		t.Fatalf("CPUPct = %v, want ~40", s.CPUPct)
	}
	if s.RxBps <= 0 || s.TxBps <= s.RxBps {
		t.Fatalf("expected positive rx and larger tx, got rx=%d tx=%d", s.RxBps, s.TxBps)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
