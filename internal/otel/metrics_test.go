package otel

import (
	"context"
	"testing"
	"time"
)

func TestDefaultMetricsConfig(t *testing.T) {
	cfg := DefaultMetricsConfig()
	if cfg == nil {
		t.Fatal("DefaultMetricsConfig returned nil")
	}
	if cfg.Enabled {
		t.Error("Expected metrics to be disabled by default")
	}
	if cfg.ServiceName != "mcpdrill" {
		t.Errorf("Expected service name 'mcpdrill', got %q", cfg.ServiceName)
	}
	if cfg.ExporterType != ExporterNone {
		t.Errorf("Expected ExporterNone, got %v", cfg.ExporterType)
	}
}

func TestNewMetrics_Disabled(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultMetricsConfig()

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	if m.Enabled() {
		t.Error("Expected metrics to be disabled")
	}
}

func TestNewMetrics_StdoutExporter(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	if !m.Enabled() {
		t.Error("Expected metrics to be enabled")
	}
}

func TestRecordOperationLatency(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	// Record some latencies
	m.RecordOperationLatency(ctx, "tools/list", "", 45.5, true)
	m.RecordOperationLatency(ctx, "tools/call", "echo", 120.3, true)
	m.RecordOperationLatency(ctx, "tools/call", "echo", 250.7, false)

	// No assertions - just verify it doesn't panic
}

func TestMetricsRecordError(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	// Record some errors
	m.RecordError(ctx, "connection_error")
	m.RecordError(ctx, "timeout")
	m.RecordError(ctx, "validation_error")

	// No assertions - just verify it doesn't panic
}

func TestSessionCounters(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	// Increment and decrement sessions
	m.IncrementSessions(ctx)
	m.IncrementSessions(ctx)
	m.IncrementSessions(ctx)
	m.DecrementSessions(ctx)

	// No assertions - just verify it doesn't panic
}

func TestReconnectAndStallCounters(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	// Record reconnects and stalls
	m.RecordReconnect(ctx)
	m.RecordReconnect(ctx)
	m.RecordStall(ctx)

	// No assertions - just verify it doesn't panic
}

func TestSetCurrentStage(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	// Set stage multiple times
	m.SetCurrentStage(0)
	m.SetCurrentStage(1)
	m.SetCurrentStage(2)

	// Verify the value is stored
	if m.currentStage.Load() != 2 {
		t.Errorf("Expected current stage 2, got %d", m.currentStage.Load())
	}
}

func TestGlobalMetrics(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	// Set and get global metrics
	SetGlobalMetrics(m)
	retrieved := GetGlobalMetrics()

	if retrieved != m {
		t.Error("GetGlobalMetrics did not return the set instance")
	}

	// Clean up
	SetGlobalMetrics(nil)
}

func TestGetGlobalMetrics_Uninitialized(t *testing.T) {
	// Ensure global is nil
	SetGlobalMetrics(nil)

	// Should return a no-op instance
	m := GetGlobalMetrics()
	if m == nil {
		t.Fatal("GetGlobalMetrics returned nil")
	}
	if m.Enabled() {
		t.Error("Expected no-op metrics to be disabled")
	}
}

func TestNoopMetrics(t *testing.T) {
	m := NoopMetrics()
	if m == nil {
		t.Fatal("NoopMetrics returned nil")
	}
	if m.Enabled() {
		t.Error("Expected no-op metrics to be disabled")
	}

	ctx := context.Background()

	// Verify all methods work without panicking
	m.RecordOperationLatency(ctx, "test", "tool", 100.0, true)
	m.RecordError(ctx, "test_error")
	m.IncrementSessions(ctx)
	m.DecrementSessions(ctx)
	m.RecordReconnect(ctx)
	m.RecordStall(ctx)
	m.SetCurrentStage(1)

	if err := m.Shutdown(ctx); err != nil {
		t.Errorf("NoopMetrics.Shutdown failed: %v", err)
	}
}

func TestMetricsShutdown(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}

	// Record some metrics
	m.RecordOperationLatency(ctx, "test", "", 50.0, true)
	m.SetCurrentStage(1)

	// Shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := m.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestMetricsWithCustomAttributes(t *testing.T) {
	ctx := context.Background()
	cfg := &MetricsConfig{
		Enabled:        true,
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		ExporterType:   ExporterStdout,
		Attributes: map[string]string{
			"environment": "test",
			"region":      "us-west-2",
		},
	}

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	if !m.Enabled() {
		t.Error("Expected metrics to be enabled")
	}
}

func TestMetricsDisabledOperations(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultMetricsConfig() // Disabled by default

	m, err := NewMetrics(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Shutdown(ctx)

	// All operations should be no-ops when disabled
	m.RecordOperationLatency(ctx, "test", "tool", 100.0, true)
	m.RecordError(ctx, "test_error")
	m.IncrementSessions(ctx)
	m.DecrementSessions(ctx)
	m.RecordReconnect(ctx)
	m.RecordStall(ctx)
	m.SetCurrentStage(1)

	// Should not panic
}
