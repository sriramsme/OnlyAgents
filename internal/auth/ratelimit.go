package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter implements LoginAttemptLimiter with a per-IP token bucket.
// Each IP gets 5 attempts, refilling at 1 attempt per minute.
// This stops brute force without blocking legitimate users who mistype.
type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
	rate     rate.Limit
	burst    int
}

type rateLimiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// NewIPRateLimiter creates a limiter allowing burst attempts,
// refilling at r per second.
// Recommended: r=rate.Every(minute), burst=5
func NewIPRateLimiter(r rate.Limit, burst int) *IPRateLimiter {
	rl := &IPRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		rate:     r,
		burst:    burst,
	}
	// Periodically clean up stale IP entries
	go rl.cleanup()
	return rl
}

func (rl *IPRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, ok := rl.limiters[ip]
	if !ok {
		entry = &rateLimiterEntry{
			limiter: rate.NewLimiter(rl.rate, rl.burst),
		}
		rl.limiters[ip] = entry
	}
	entry.lastAccess = time.Now()
	return entry.limiter.Allow()
}

func (rl *IPRateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, entry := range rl.limiters {
			if time.Since(entry.lastAccess) > 1*time.Hour {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}
