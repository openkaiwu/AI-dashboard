package httpx

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Bounded per-IP authentication throttle. Reverse proxies should add their own edge limit.
func Guard(next http.Handler) http.Handler {
	type budget struct {
		Start time.Time
		Count int
	}
	var mu sync.Mutex
	entries := map[string]budget{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if r.Method == "POST" && (r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/register") {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			now := time.Now()
			mu.Lock()
			for key, v := range entries {
				if now.Sub(v.Start) > time.Minute {
					delete(entries, key)
				}
			}
			b, exists := entries[ip]
			if !exists {
				b = budget{Start: now}
			}
			blocked := b.Count >= 20 || (!exists && len(entries) >= 4096)
			if !blocked {
				b.Count++
				entries[ip] = b
			}
			mu.Unlock()
			if blocked {
				w.Header().Set("Retry-After", "60")
				Error(w, 429, "rate_limited", "请稍后重试")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
