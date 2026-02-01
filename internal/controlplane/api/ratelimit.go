package api

import (
	"log"
	"sync"
	"time"
)

const (
	defaultMaxRateLimiterClients      = 10000
	defaultRateLimiterClientTTL       = 10 * time.Minute
	defaultRateLimiterCleanupInterval = time.Minute
)

// RateLimiterConfig configures the token bucket rate limiter.
type RateLimiterConfig struct {
	// RequestsPerSecond is the rate at which tokens are added to the bucket.
	RequestsPerSecond float64
	// BurstSize is the maximum number of tokens (burst capacity).
	BurstSize int
	// Enabled controls whether rate limiting is active.
	Enabled bool
	// MaxClients is the maximum number of client buckets to retain.
	MaxClients int
	// ClientTTL is how long to retain idle client buckets.
	ClientTTL time.Duration
	// CleanupInterval controls how often idle buckets are cleaned.
	CleanupInterval time.Duration
}

// DefaultRateLimiterConfig returns sensible defaults for the rate limiter.
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		RequestsPerSecond: 100, // 100 requests/second sustained
		BurstSize:         200, // Allow bursts up to 200 requests
		Enabled:           true,
		MaxClients:        defaultMaxRateLimiterClients,
		ClientTTL:         defaultRateLimiterClientTTL,
		CleanupInterval:   defaultRateLimiterCleanupInterval,
	}
}

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	mu           sync.Mutex
	tokens       float64
	maxTokens    float64
	refillRate   float64 // tokens per nanosecond
	lastRefillNs int64
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
	config      *RateLimiterConfig
	mu          sync.Mutex
	buckets     map[string]*clientBucket
	lastCleanup time.Time
}

type clientBucket struct {
	bucket   *tokenBucket
	lastSeen time.Time
}

func newRateLimiter(config *RateLimiterConfig) *rateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}
	return &rateLimiter{
		config:      config,
		buckets:     make(map[string]*clientBucket),
		lastCleanup: time.Now(),
	}
}

// allow checks if a request should be allowed.
func (rl *rateLimiter) allow() bool {
	return rl.allowKey("global")
}

func (rl *rateLimiter) allowKey(key string) bool {
	if !rl.config.Enabled {
		return true
	}
	if key == "" {
		key = "unknown"
	}

	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.cleanupLocked(now)

	bucket, ok := rl.buckets[key]
	if !ok {
		if rl.config.MaxClients > 0 && len(rl.buckets) >= rl.config.MaxClients {
			rl.evictOldestLocked()
		}
		bucket = &clientBucket{
			bucket:   newTokenBucket(rl.config.RequestsPerSecond, rl.config.BurstSize),
			lastSeen: now,
		}
		rl.buckets[key] = bucket
	}

	bucket.lastSeen = now
	return bucket.bucket.take()
}

func (rl *rateLimiter) cleanupLocked(now time.Time) {
	interval := rl.config.CleanupInterval
	if interval <= 0 {
		interval = defaultRateLimiterCleanupInterval
	}
	if now.Sub(rl.lastCleanup) < interval {
		return
	}
	rl.lastCleanup = now

	ttl := rl.config.ClientTTL
	if ttl <= 0 {
		ttl = defaultRateLimiterClientTTL
	}
	cutoff := now.Add(-ttl)
	for key, bucket := range rl.buckets {
		if bucket.lastSeen.Before(cutoff) {
			delete(rl.buckets, key)
		}
	}
}

func (rl *rateLimiter) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for key, bucket := range rl.buckets {
		if first || bucket.lastSeen.Before(oldestTime) {
			oldestKey = key
			oldestTime = bucket.lastSeen
			first = false
		}
	}
	if oldestKey != "" {
		log.Printf("[RateLimiter] Max clients reached (%d). Evicting oldest bucket: %s", rl.config.MaxClients, oldestKey)
		delete(rl.buckets, oldestKey)
	}
}
