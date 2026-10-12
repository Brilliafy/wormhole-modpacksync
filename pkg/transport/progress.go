package transport

import (
	"sync"
	"time"
)

// progressThrottle rate-limits UI status updates. Chunk-level progress used to
// emit one event per 64KB read, which flooded the WebView with hundreds of
// thousands of events on a 10GB transfer and starved the UI thread.
type progressThrottle struct {
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

func newProgressThrottle(interval time.Duration) *progressThrottle {
	return &progressThrottle{
		interval: interval,
		last:     time.Time{}, // First call always passes
	}
}

// allow reports whether a new progress event may be emitted now.
func (t *progressThrottle) allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.last) >= t.interval {
		t.last = now
		return true
	}
	return false
}
