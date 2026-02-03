package vu

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type RateLimiter struct {
	targetRPS  atomic.Value
	tokens     float64
	maxTokens  float64
	lastRefill time.Time
	refillRate float64
	mu         sync.Mutex
	enabled    atomic.Bool
}

func NewRateLimiter(targetRPS float64) *RateLimiter {
	r := &RateLimiter{}
	r.targetRPS.Store(targetRPS)

	if targetRPS <= 0 {
		r.enabled.Store(false)
		return r
	}

	maxTokens := targetRPS
	if maxTokens < 1 {
		maxTokens = 1
	}
	if maxTokens > 10000 {
		maxTokens = 10000
	}

	r.tokens = maxTokens
	r.maxTokens = maxTokens
	r.lastRefill = time.Now()
	r.refillRate = targetRPS
	r.enabled.Store(true)

	return r
}

func (r *RateLimiter) Acquire(ctx context.Context) error {
	if !r.enabled.Load() {
		return nil
	}

	for {
		waitDuration, done := func() (time.Duration, bool) {
			r.mu.Lock()
			defer r.mu.Unlock()

			if !r.enabled.Load() {
				return 0, true
			}

			r.refill()

			if r.tokens >= 1 {
				r.tokens--
				return 0, true
			}

			waitDuration := time.Duration(float64(time.Second) / r.refillRate)
			if waitDuration < 100*time.Microsecond {
				waitDuration = 100 * time.Microsecond
			}
			return waitDuration, false
		}()

		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
		}
	}
}

func (r *RateLimiter) TryAcquire() bool {
	if !r.enabled.Load() {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.enabled.Load() {
		return true
	}

	r.refill()

	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

func (r *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.lastRefill = now

	r.tokens += elapsed * r.refillRate
	if r.tokens > r.maxTokens {
		r.tokens = r.maxTokens
	}
}

func (r *RateLimiter) TargetRPS() float64 {
	return r.targetRPS.Load().(float64)
}

func (r *RateLimiter) Enabled() bool {
	return r.enabled.Load()
}

func (r *RateLimiter) AvailableTokens() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refill()
	return r.tokens
}

func (r *RateLimiter) UpdateTargetRPS(targetRPS float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.targetRPS.Store(targetRPS)

	if targetRPS <= 0 {
		r.enabled.Store(false)
		return
	}

	r.enabled.Store(true)
	r.refillRate = targetRPS

	maxTokens := targetRPS
	if maxTokens < 1 {
		maxTokens = 1
	}
	if maxTokens > 10000 {
		maxTokens = 10000
	}
	r.maxTokens = maxTokens

	if r.tokens > r.maxTokens {
		r.tokens = r.maxTokens
	}
}

type InFlightLimiter struct {
	maxInFlight int
	current     int
	mu          sync.Mutex
	cond        *sync.Cond
}

func NewInFlightLimiter(maxInFlight int) *InFlightLimiter {
	l := &InFlightLimiter{
		maxInFlight: maxInFlight,
	}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func (l *InFlightLimiter) Acquire(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.current >= l.maxInFlight {
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				l.cond.Broadcast()
			case <-done:
			}
		}()
		defer close(done)

		for l.current >= l.maxInFlight {
			l.cond.Wait()

			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}

	l.current++
	return nil
}

func (l *InFlightLimiter) TryAcquire() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.current >= l.maxInFlight {
		return false
	}

	l.current++
	return true
}

func (l *InFlightLimiter) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.current > 0 {
		l.current--
	}
	l.cond.Signal()
}

func (l *InFlightLimiter) Current() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.current
}

func (l *InFlightLimiter) MaxInFlight() int {
	return l.maxInFlight
}
