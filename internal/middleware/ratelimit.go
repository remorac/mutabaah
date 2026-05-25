package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimit returns a middleware that allows at most `burst` requests per IP,
// refilling at `refillPer` interval. Designed for low-volume endpoints like
// /login; not a substitute for a real edge limiter.
//
// The store is an in-memory map keyed by IP, swept periodically to prevent
// unbounded growth. State is per-process: a multi-instance deployment will
// effectively raise the limit by the instance count, which is acceptable for
// brute-force login defense.
func RateLimit(burst int, refillPer time.Duration) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		burst:     burst,
		refillPer: refillPer,
		buckets:   make(map[string]*bucket),
	}
	go rl.sweepLoop()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.allow(clientIP(r)) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateLimiter struct {
	burst     int
	refillPer time.Duration
	mu        sync.Mutex
	buckets   map[string]*bucket
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(rl.burst), lastSeen: now}
		rl.buckets[key] = b
	}
	elapsed := now.Sub(b.lastSeen).Seconds()
	refillRate := 1.0 / rl.refillPer.Seconds()
	b.tokens = minF(float64(rl.burst), b.tokens+elapsed*refillRate)
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *rateLimiter) sweepLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-30 * time.Minute)
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
