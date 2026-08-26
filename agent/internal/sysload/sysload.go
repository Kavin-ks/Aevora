// Package sysload reads real host load on a Linux gateway: CPU utilisation from
// /proc/stat and WAN throughput from the interface byte counters. Rates are
// computed as deltas between successive Sample() calls. The file readers are
// injectable so the delta logic is unit-testable without /proc.
package sysload

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Sample is one load reading. RxBps/TxBps are in bits per second (to match the
// gateway's advertised bandwidth in megabits).
type Sample struct {
	CPUPct float64
	RxBps  int64
	TxBps  int64
}

type cpuSample struct{ total, idle uint64 }
type netSample struct {
	rx, tx uint64
	at     time.Time
}

// Meter computes load deltas across calls.
type Meter struct {
	wanIface     string
	readProcStat func() (string, error)
	readNetBytes func(iface string) (rx, tx uint64, err error)

	prevCPU cpuSample
	prevNet netSample
	hasCPU  bool
	hasNet  bool
}

// New returns a Meter for the given WAN interface using the real /proc and /sys.
func New(wanIface string) *Meter {
	return &Meter{
		wanIface:     wanIface,
		readProcStat: defaultReadProcStat,
		readNetBytes: defaultReadNetBytes,
	}
}

// Sample returns the load since the previous call. The first call establishes a
// baseline and returns zeros.
func (m *Meter) Sample() Sample {
	var s Sample
	now := time.Now()

	if content, err := m.readProcStat(); err == nil {
		total, idle := parseProcStat(content)
		if total > 0 {
			if m.hasCPU {
				dt := int64(total) - int64(m.prevCPU.total)
				di := int64(idle) - int64(m.prevCPU.idle)
				if dt > 0 {
					s.CPUPct = float64(dt-di) / float64(dt) * 100
				}
			}
			m.prevCPU = cpuSample{total, idle}
			m.hasCPU = true
		}
	}

	if rx, tx, err := m.readNetBytes(m.wanIface); err == nil {
		if m.hasNet {
			dt := now.Sub(m.prevNet.at).Seconds()
			if dt > 0 {
				if rx >= m.prevNet.rx {
					s.RxBps = int64(float64(rx-m.prevNet.rx) * 8 / dt)
				}
				if tx >= m.prevNet.tx {
					s.TxBps = int64(float64(tx-m.prevNet.tx) * 8 / dt)
				}
			}
		}
		m.prevNet = netSample{rx, tx, now}
		m.hasNet = true
	}

	return s
}

// parseProcStat parses the aggregate "cpu" line of /proc/stat into total and
// idle jiffies. Returns (0,0) if not found.
func parseProcStat(content string) (total, idle uint64) {
	for _, line := range strings.Split(content, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || f[0] != "cpu" {
			continue
		}
		for i, v := range f[1:] {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				continue
			}
			total += n
			// Fields: user nice system idle iowait irq softirq steal ...
			if i == 3 || i == 4 { // idle + iowait
				idle += n
			}
		}
		return total, idle
	}
	return 0, 0
}

func defaultReadProcStat() (string, error) {
	b, err := os.ReadFile("/proc/stat")
	return string(b), err
}

func defaultReadNetBytes(iface string) (rx, tx uint64, err error) {
	base := fmt.Sprintf("/sys/class/net/%s/statistics/", iface)
	if rx, err = readUint(base + "rx_bytes"); err != nil {
		return 0, 0, err
	}
	if tx, err = readUint(base + "tx_bytes"); err != nil {
		return 0, 0, err
	}
	return rx, tx, nil
}

func readUint(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}
