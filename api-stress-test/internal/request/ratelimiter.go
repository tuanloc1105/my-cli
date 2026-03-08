package request

import (
	"context"
	"time"
)

// RateLimiter provides a simple token-bucket rate limiter using time.Ticker.
type RateLimiter struct {
	ticker *time.Ticker
}

// NewRateLimiter creates a rate limiter that allows rps requests per second.
// If rps is <= 0, returns a no-op limiter (unlimited).
func NewRateLimiter(rps float64) *RateLimiter {
	if rps <= 0 {
		return &RateLimiter{}
	}
	interval := time.Duration(float64(time.Second) / rps)
	return &RateLimiter{ticker: time.NewTicker(interval)}
}

// Wait blocks until the next request is allowed or context is cancelled.
// Returns true if allowed, false if context was cancelled.
// No-op (returns true) if rate limiting is disabled.
func (r *RateLimiter) Wait(ctx context.Context) bool {
	if r.ticker == nil {
		return ctx.Err() == nil
	}
	select {
	case <-r.ticker.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Stop releases the rate limiter resources.
func (r *RateLimiter) Stop() {
	if r.ticker != nil {
		r.ticker.Stop()
	}
}
