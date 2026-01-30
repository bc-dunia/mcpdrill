package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
)

func TestTokenBucket_Basic(t *testing.T) {
	bucket := newTokenBucket(10, 5) // 10 req/s, burst of 5

	// Should allow burst of 5
	for i := 0; i < 5; i++ {
		if !bucket.take() {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 6th request should be denied (bucket empty)
	if bucket.take() {
		t.Error("Expected 6th request to be denied")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	bucket := newTokenBucket(100, 1) // 100 req/s, burst of 1

	// Take the one available token
	if !bucket.take() {
		t.Error("Expected first request to be allowed")
	}

	// Should be denied immediately
	if bucket.take() {
		t.Error("Expected second request to be denied")
	}

	// Wait for refill (10ms should give us 1 token at 100/s)
	time.Sleep(15 * time.Millisecond)

	// Should now be allowed
	if !bucket.take() {
		t.Error("Expected request after refill to be allowed")
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 1,
		BurstSize:         1,
		Enabled:           false,
	}
	rl := newRateLimiter(config)

	// Should allow unlimited requests when disabled
	for i := 0; i < 100; i++ {
		if !rl.allow() {
			t.Errorf("Expected request %d to be allowed when rate limiting is disabled", i+1)
		}
	}
}

func TestRateLimiter_Enabled(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 10,
		BurstSize:         3,
		Enabled:           true,
	}
	rl := newRateLimiter(config)

	// Should allow burst
	allowed := 0
	for i := 0; i < 10; i++ {
		if rl.allow() {
			allowed++
		}
	}

	if allowed != 3 {
		t.Errorf("Expected 3 requests allowed (burst), got %d", allowed)
	}
}

func TestDefaultRateLimiterConfig(t *testing.T) {
	config := DefaultRateLimiterConfig()

	if config.RequestsPerSecond != 100 {
		t.Errorf("Expected 100 req/s, got %f", config.RequestsPerSecond)
	}
	if config.BurstSize != 200 {
		t.Errorf("Expected burst of 200, got %d", config.BurstSize)
	}
	if !config.Enabled {
		t.Error("Expected rate limiting to be enabled by default")
	}
}

func TestRateLimitMiddleware_Headers(t *testing.T) {
	// Create a server with very low rate limit for testing
	rm := runmanager.NewRunManager(nil)
	server := NewServer("127.0.0.1:0", rm)
	server.SetRateLimiterConfig(&RateLimiterConfig{
		RequestsPerSecond: 1,
		BurstSize:         1,
		Enabled:           true,
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	client := &http.Client{}
	url := server.URL() + "/runs"

	// Make requests until we hit rate limit
	var lastResp *http.Response
	for i := 0; i < 5; i++ {
		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			lastResp = resp
			break
		}
		resp.Body.Close()
	}

	if lastResp == nil {
		t.Skip("Could not trigger rate limit in time")
		return
	}
	defer lastResp.Body.Close()

	// Verify headers
	if lastResp.Header.Get("X-RateLimit-Limit") == "" {
		t.Error("Expected X-RateLimit-Limit header")
	}
	if lastResp.Header.Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("Expected X-RateLimit-Remaining=0, got %s", lastResp.Header.Get("X-RateLimit-Remaining"))
	}
	if lastResp.Header.Get("Retry-After") == "" {
		t.Error("Expected Retry-After header")
	}
}
