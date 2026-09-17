package license

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu      sync.Mutex
	attempt map[string][]time.Time
	limit   int
	window  time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempt: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

func (r *RateLimiter) prune(key string, now time.Time) []time.Time {
	cutoff := now.Add(-r.window)
	prev := r.attempt[key]
	var kept []time.Time
	for _, t := range prev {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(r.attempt, key)
	} else {
		r.attempt[key] = kept
	}
	return kept
}

func (r *RateLimiter) Exceeded(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	hits := r.prune(key, now)
	return len(hits) >= r.limit
}

func (r *RateLimiter) RecordFailure(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.prune(key, now)
	r.attempt[key] = append(r.attempt[key], now)
}

func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attempt, key)
}
