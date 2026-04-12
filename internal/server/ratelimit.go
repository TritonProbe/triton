package server

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type rateLimiter struct {
	mu           sync.Mutex
	limit        int
	buckets      map[string]bucket
	now          func() time.Time
	sweepEvery   int
	requestsSeen int
	maxIdle      time.Duration
}

type bucket struct {
	window   int64
	count    int
	lastSeen time.Time
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{
		limit:      limit,
		buckets:    make(map[string]bucket),
		now:        time.Now,
		sweepEvery: 256,
		maxIdle:    2 * time.Minute,
	}
}

func (r *rateLimiter) middleware(next http.Handler) http.Handler {
	if r == nil || r.limit <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ip := clientIP(req.RemoteAddr)
		now := r.now()
		window := now.Unix()

		r.mu.Lock()
		r.requestsSeen++
		if r.sweepEvery > 0 && r.requestsSeen%r.sweepEvery == 0 {
			r.sweepLocked(now)
		}
		entry := r.buckets[ip]
		if entry.window != window {
			entry.window = window
			entry.count = 0
		}
		entry.count++
		entry.lastSeen = now
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

func (r *rateLimiter) sweepLocked(now time.Time) {
	for ip, entry := range r.buckets {
		if now.Sub(entry.lastSeen) > r.maxIdle {
			delete(r.buckets, ip)
		}
	}
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil || host == "" {
		return remoteAddr
	}
	return host
}
