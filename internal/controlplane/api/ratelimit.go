package api

import (
	"sync"
	"time"
)

// RateLimiterConfig configures the token bucket rate limiter.
type RateLimiterConfig struct {
	// RequestsPerSecond is the rate at which tokens are added to the bucket.
	RequestsPerSecond float64
	// BurstSize is the maximum number of tokens (burst capacity).
	BurstSize int
	// Enabled controls whether rate limiting is active.
	Enabled bool
}

// DefaultRateLimiterConfig returns sensible defaults for the rate limiter.
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		RequestsPerSecond: 100, // 100 requests/second sustained
		BurstSize:         200, // Allow bursts up to 200 requests
		Enabled:           true,
	}
}

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	mu            sync.Mutex
	tokens        float64
	maxTokens     float64
	refillRate    float64 // tokens per nanosecond
	lastRefillNs  int64
}

func newTokenBucket(requestsPerSecond float64, burstSize int) *tokenBucket {
	return &tokenBucket{
		tokens:       float64(burstSize), // Start full
		maxTokens:    float64(burstSize),
		refillRate:   requestsPerSecond / float64(time.Second), // tokens per nanosecond
		lastRefillNs: time.Now().UnixNano(),
	}
}

// take attempts to take a token from the bucket.
// Returns true if successful, false if rate limited.
func (tb *tokenBucket) take() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now().UnixNano()
	elapsed := now - tb.lastRefillNs
	tb.lastRefillNs = now

	// Refill tokens based on elapsed time
	tb.tokens += float64(elapsed) * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}

	// Try to take a token
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// rateLimiter manages rate limiting state.
type rateLimiter struct {
	config *RateLimiterConfig
	bucket *tokenBucket
}

func newRateLimiter(config *RateLimiterConfig) *rateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}
	return &rateLimiter{
		config: config,
		bucket: newTokenBucket(config.RequestsPerSecond, config.BurstSize),
	}
}

// allow checks if a request should be allowed.
func (rl *rateLimiter) allow() bool {
	if !rl.config.Enabled {
		return true
	}
	return rl.bucket.take()
}
