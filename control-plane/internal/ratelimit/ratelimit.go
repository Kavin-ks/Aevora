// Package ratelimit provides a simple per-key token-bucket limiter, used to
// throttle abuse-prone endpoints (login, enroll, refresh) per client IP.
//
// It is in-memory and therefore per-instance; behind multiple control-plane
// replicas it limits per replica. That is acceptable as a first line of defence
// (a shared limiter would use Redis); document the deployment assumption.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Limiter throttles events per key (e.g. client IP).
type Limiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     rate.Limit
	burst    int
	ttl      time.Duration
}

// New builds a limiter allowing `perMinute` events per key with the given burst.
func New(perMinute, burst int) *Limiter {
	l := &Limiter{
		visitors: make(map[string]*visitor),
		rate:     rate.Every(time.Minute / time.Duration(max(perMinute, 1))),
		burst:    max(burst, 1),
		ttl:      10 * time.Minute,
	}
	return l
}

// Allow reports whether an event for key is permitted now.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.visitors[key]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.visitors[key] = v
	}
	v.lastSeen = time.Now()
	return v.limiter.Allow()
}

// Cleanup evicts keys not seen within the TTL. Call periodically.
func (l *Limiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-l.ttl)
	for k, v := range l.visitors {
		if v.lastSeen.Before(cutoff) {
			delete(l.visitors, k)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
