package vu

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/session"
	"github.com/bc-dunia/mcpdrill/internal/transport"
)

type UserJourneyExecutor struct {
	config            *UserJourneyConfig
	conn              transport.Connection
	consecutiveErrors atomic.Int32
	lastToolsListTime atomic.Int64
	retryState        *retryState
	rng               *rand.Rand
	mu                sync.Mutex
}

type retryState struct {
	attempts  int
	nextDelay int64
	lastError error
}

func NewUserJourneyExecutor(config *UserJourneyConfig, seed int64) *UserJourneyExecutor {
	if config == nil {
		config = DefaultUserJourneyConfig()
	}
	return &UserJourneyExecutor{
		config:     config,
		retryState: &retryState{},
		rng:        rand.New(rand.NewSource(seed)),
	}
}

func (e *UserJourneyExecutor) RunStartupSequence(
	ctx context.Context,
	sess *session.SessionInfo,
) (*transport.OperationOutcome, error) {
	if e.config == nil || e.config.StartupSequence == nil {
		return nil, nil
	}

	if !e.config.StartupSequence.RunToolsListOnStart {
		return nil, nil
	}

	conn := sess.Connection
	if conn == nil {
		return nil, nil
	}

	outcome, err := conn.ToolsList(ctx, nil)
	if err != nil {
		return outcome, err
	}

	e.lastToolsListTime.Store(time.Now().UnixMilli())
	return outcome, nil
}

func (e *UserJourneyExecutor) ShouldRunPeriodicToolsList() bool {
	if e.config == nil || e.config.PeriodicOps == nil {
		return false
	}

	intervalMs := e.config.PeriodicOps.ToolsListIntervalMs
	if intervalMs <= 0 {
		return false
	}

	lastTime := e.lastToolsListTime.Load()
	if lastTime == 0 {
		return true
	}

	elapsed := time.Now().UnixMilli() - lastTime
	return elapsed >= intervalMs
}

func (e *UserJourneyExecutor) ShouldRunToolsListAfterErrors() bool {
	if e.config == nil || e.config.PeriodicOps == nil {
		return false
	}

	threshold := e.config.PeriodicOps.ToolsListAfterErrors
	if threshold <= 0 {
		return false
	}

	return int(e.consecutiveErrors.Load()) >= threshold
}

func (e *UserJourneyExecutor) RunPeriodicToolsList(
	ctx context.Context,
	sess *session.SessionInfo,
) (*transport.OperationOutcome, error) {
	conn := sess.Connection
	if conn == nil {
		return nil, nil
	}

	outcome, err := conn.ToolsList(ctx, nil)
	if err == nil && outcome != nil && outcome.OK {
		e.lastToolsListTime.Store(time.Now().UnixMilli())
		e.consecutiveErrors.Store(0)
	}
	return outcome, err
}

func (e *UserJourneyExecutor) RecordOperationResult(ok bool) {
	if ok {
		e.consecutiveErrors.Store(0)
	} else {
		e.consecutiveErrors.Add(1)
	}
}

func (e *UserJourneyExecutor) ConsecutiveErrors() int {
	return int(e.consecutiveErrors.Load())
}

func (e *UserJourneyExecutor) ShouldRetryReconnect() bool {
	if e.config == nil || e.config.ReconnectPolicy == nil {
		return false
	}
	if !e.config.ReconnectPolicy.Enabled {
		return false
	}

	maxRetries := e.config.ReconnectPolicy.MaxRetries
	if maxRetries > 0 && e.retryState.attempts >= maxRetries {
		return false
	}

	return true
}

func (e *UserJourneyExecutor) GetReconnectDelay() time.Duration {
	if e.config == nil || e.config.ReconnectPolicy == nil {
		return 0
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	policy := e.config.ReconnectPolicy

	if e.retryState.attempts == 0 {
		e.retryState.nextDelay = policy.InitialDelayMs
	}

	delay := e.retryState.nextDelay

	if policy.JitterFraction > 0 {
		jitterRange := float64(delay) * policy.JitterFraction
		jitter := e.rng.Float64()*jitterRange*2 - jitterRange
		delay = int64(math.Max(0, float64(delay)+jitter))
	}

	nextDelay := int64(float64(e.retryState.nextDelay) * policy.Multiplier)
	if nextDelay > policy.MaxDelayMs {
		nextDelay = policy.MaxDelayMs
	}
	e.retryState.nextDelay = nextDelay
	e.retryState.attempts++

	return time.Duration(delay) * time.Millisecond
}

func (e *UserJourneyExecutor) ResetRetryState() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.retryState.attempts = 0
	e.retryState.nextDelay = 0
	e.retryState.lastError = nil
}

func (e *UserJourneyExecutor) RetryAttempts() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.retryState.attempts
}
