package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// ipLimiter is a fixed-window request counter per remote address. It lives in
// process on purpose: the server targets single-instance self-hosted
// deployments, so no cross-node coordination is needed.
type ipLimiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	hits   map[string]*ipWindow
	lastGC time.Time
}

type ipWindow struct {
	start time.Time
	count int
}

func newIPLimiter(max int, window time.Duration) *ipLimiter {
	return &ipLimiter{window: window, max: max, hits: map[string]*ipWindow{}, lastGC: time.Now()}
}

func (l *ipLimiter) Allow(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastGC) > l.window {
		for key, win := range l.hits {
			if now.Sub(win.start) >= l.window {
				delete(l.hits, key)
			}
		}
		l.lastGC = now
	}
	win, ok := l.hits[host]
	if !ok || now.Sub(win.start) >= l.window {
		l.hits[host] = &ipWindow{start: now, count: 1}
		return true
	}
	if win.count >= l.max {
		return false
	}
	win.count++
	return true
}
