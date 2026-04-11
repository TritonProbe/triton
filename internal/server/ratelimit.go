package server

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	buckets map[string]bucket
}

type bucket struct {
	window int64
	count  int
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{
		limit:   limit,
		buckets: make(map[string]bucket),
	}
}

func (r *rateLimiter) middleware(next http.Handler) http.Handler {
	if r == nil || r.limit <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ip := clientIP(req.RemoteAddr)
		now := time.Now().Unix()

		r.mu.Lock()
		entry := r.buckets[ip]
		if entry.window != now {
			entry.window = now
			entry.count = 0
		}
		entry.count++
		r.buckets[ip] = entry
		allowed := entry.count <= r.limit
		r.mu.Unlock()

		if !allowed {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil || host == "" {
		return remoteAddr
	}
	return host
}
