package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	loginAttempts   = 10
	loginWindow     = 15 * time.Minute
	limiterCapacity = 4096
)

// limiter is a fixed-window counter. It only guards the login endpoint, so the
// memory it needs is bounded by the number of distinct clients inside a window.
type limiter struct {
	limit  int
	period time.Duration

	mu      sync.Mutex
	windows map[string]*limiterWindow
}

func newLimiter(limit int, period time.Duration) *limiter {
	return &limiter{limit: limit, period: period, windows: map[string]*limiterWindow{}}
}

type limiterWindow struct {
	count int
	reset time.Time
}

// allow records an attempt, reporting how long the caller must wait when the window is full.
func (l *limiter) allow(key string) (bool, time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.windows) > limiterCapacity {
		l.sweep(now)
	}

	window, found := l.windows[key]
	if !found || now.After(window.reset) {
		l.windows[key] = &limiterWindow{count: 1, reset: now.Add(l.period)}

		return true, 0
	}

	if window.count >= l.limit {
		return false, window.reset.Sub(now)
	}

	window.count++

	return true, 0
}

// forget clears a key, so a successful login does not count against the client.
func (l *limiter) forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.windows, key)
}

func (l *limiter) sweep(now time.Time) {
	for key, window := range l.windows {
		if now.After(window.reset) {
			delete(l.windows, key)
		}
	}
}

// clientIP prefers the socket address: forwarded headers are caller supplied and trivially spoofed.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
