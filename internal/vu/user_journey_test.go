package vu

import (
	"testing"
	"time"
)

func TestUserJourneyExecutor_ReconnectPolicy(t *testing.T) {
	config := &UserJourneyConfig{
		ReconnectPolicy: &ReconnectPolicyConfig{
			Enabled:        true,
			InitialDelayMs: 100,
			MaxDelayMs:     1000,
			Multiplier:     2.0,
			JitterFraction: 0.0,
			MaxRetries:     5,
		},
	}

	executor := NewUserJourneyExecutor(config, 12345)

	if !executor.ShouldRetryReconnect() {
		t.Error("Expected ShouldRetryReconnect to return true initially")
	}

	delays := []int64{100, 200, 400, 800, 1000}
	for i, expectedDelay := range delays {
		delay := executor.GetReconnectDelay()
		actualMs := delay.Milliseconds()
		if actualMs != expectedDelay {
			t.Errorf("Retry %d: expected delay %d ms, got %d ms", i+1, expectedDelay, actualMs)
		}
	}

	if executor.ShouldRetryReconnect() {
		t.Error("Expected ShouldRetryReconnect to return false after max retries")
	}

	executor.ResetRetryState()
	if !executor.ShouldRetryReconnect() {
		t.Error("Expected ShouldRetryReconnect to return true after reset")
	}
}

func TestUserJourneyExecutor_ReconnectPolicyWithJitter(t *testing.T) {
	config := &UserJourneyConfig{
		ReconnectPolicy: &ReconnectPolicyConfig{
			Enabled:        true,
			InitialDelayMs: 100,
			MaxDelayMs:     1000,
			Multiplier:     2.0,
			JitterFraction: 0.2,
			MaxRetries:     3,
		},
	}

	executor := NewUserJourneyExecutor(config, 12345)

	delay := executor.GetReconnectDelay()
	actualMs := delay.Milliseconds()

	minExpected := int64(80)
	maxExpected := int64(120)
	if actualMs < minExpected || actualMs > maxExpected {
		t.Errorf("First delay %d ms should be within [%d, %d] ms with 20%% jitter",
			actualMs, minExpected, maxExpected)
	}
}

func TestUserJourneyExecutor_ConsecutiveErrors(t *testing.T) {
	config := &UserJourneyConfig{
		PeriodicOps: &PeriodicOpsConfig{
			ToolsListAfterErrors: 3,
		},
	}

	executor := NewUserJourneyExecutor(config, 12345)

	if executor.ShouldRunToolsListAfterErrors() {
		t.Error("Should not trigger tools/list with 0 errors")
	}

	executor.RecordOperationResult(false)
	executor.RecordOperationResult(false)
	if executor.ShouldRunToolsListAfterErrors() {
		t.Error("Should not trigger tools/list with 2 errors")
	}

	executor.RecordOperationResult(false)
	if !executor.ShouldRunToolsListAfterErrors() {
		t.Error("Should trigger tools/list after 3 consecutive errors")
	}

	executor.RecordOperationResult(true)
	if executor.ShouldRunToolsListAfterErrors() {
		t.Error("Should reset error count after success")
	}
}

func TestUserJourneyExecutor_PeriodicToolsList(t *testing.T) {
	config := &UserJourneyConfig{
		PeriodicOps: &PeriodicOpsConfig{
			ToolsListIntervalMs: 100,
		},
	}

	executor := NewUserJourneyExecutor(config, 12345)

	if !executor.ShouldRunPeriodicToolsList() {
		t.Error("Should trigger tools/list when lastToolsListTime is 0")
	}

	executor.lastToolsListTime.Store(time.Now().UnixMilli())

	if executor.ShouldRunPeriodicToolsList() {
		t.Error("Should not trigger tools/list immediately after recording")
	}

	time.Sleep(150 * time.Millisecond)

	if !executor.ShouldRunPeriodicToolsList() {
		t.Error("Should trigger tools/list after interval elapsed")
	}
}

func TestUserJourneyExecutor_DisabledReconnect(t *testing.T) {
	config := &UserJourneyConfig{
		ReconnectPolicy: &ReconnectPolicyConfig{
			Enabled: false,
		},
	}

	executor := NewUserJourneyExecutor(config, 12345)

	if executor.ShouldRetryReconnect() {
		t.Error("Should not retry when reconnect policy is disabled")
	}
}

func TestUserJourneyExecutor_NilConfig(t *testing.T) {
	executor := NewUserJourneyExecutor(nil, 12345)

	if !executor.ShouldRetryReconnect() {
		t.Error("Default config should enable reconnect")
	}

	executor.RecordOperationResult(false)
	executor.RecordOperationResult(false)
	executor.RecordOperationResult(false)
	if !executor.ShouldRunToolsListAfterErrors() {
		t.Error("Default config should trigger tools/list after errors")
	}
}

func TestDefaultUserJourneyConfig(t *testing.T) {
	config := DefaultUserJourneyConfig()

	if config == nil {
		t.Fatal("DefaultUserJourneyConfig returned nil")
	}

	if config.StartupSequence == nil || !config.StartupSequence.RunToolsListOnStart {
		t.Error("Default should enable tools/list on start")
	}

	if config.PeriodicOps == nil {
		t.Fatal("PeriodicOps should not be nil")
	}

	if config.PeriodicOps.ToolsListIntervalMs != DefaultToolsListIntervalMs {
		t.Errorf("Expected ToolsListIntervalMs %d, got %d",
			DefaultToolsListIntervalMs, config.PeriodicOps.ToolsListIntervalMs)
	}

	if config.ReconnectPolicy == nil {
		t.Fatal("ReconnectPolicy should not be nil")
	}

	if !config.ReconnectPolicy.Enabled {
		t.Error("Default reconnect policy should be enabled")
	}

	if config.ReconnectPolicy.MaxRetries != DefaultReconnectMaxRetries {
		t.Errorf("Expected MaxRetries %d, got %d",
			DefaultReconnectMaxRetries, config.ReconnectPolicy.MaxRetries)
	}
}
