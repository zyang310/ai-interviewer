package server

import (
	"sync"
	"time"
)

// limiter is an in-memory sliding-window rate limiter: at most max events per
// window per key. It suits a single-instance deployment (Cloud Run with
// --max-instances 1, which this service uses); a multi-instance deployment
// would need shared state (e.g. Redis). Keys accumulate for the process
// lifetime — negligible at test-phase volume on a service that scales to zero.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	events map[string][]time.Time
}

// newLimiter builds a limiter allowing max events per window.
func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{
		max:    max,
		window: window,
		events: make(map[string][]time.Time),
	}
}

// allow records an event for key now and reports whether it is within the
// limit. Timestamps older than the window are pruned on each call.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Filter in place: kept shares the backing array, and the write index never
	// outruns the read index, so this is safe.
	kept := l.events[key][:0]
	for _, t := range l.events[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= l.max {
		l.events[key] = kept
		return false
	}
	l.events[key] = append(kept, now)
	return true
}
