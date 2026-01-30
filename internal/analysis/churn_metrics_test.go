package analysis

import (
	"sync"
	"testing"
)

func TestChurnMetricsNoSamples(t *testing.T) {
	agg := NewAggregator()
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})

	metrics := agg.Compute()

	if metrics.ChurnMetrics != nil {
		t.Error("expected nil churn metrics when no samples")
	}
}

func TestChurnMetricsBasic(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(0, 10000)

	agg.AddChurnSample(ChurnSample{
		SessionsCreated:   5,
		SessionsDestroyed: 3,
		ActiveSessions:    2,
		ReconnectAttempts: 1,
	})

	metrics := agg.Compute()

	if metrics.ChurnMetrics == nil {
		t.Fatal("expected churn metrics")
	}
	if metrics.ChurnMetrics.SessionsCreated != 5 {
		t.Errorf("expected 5 sessions created, got %d", metrics.ChurnMetrics.SessionsCreated)
	}
	if metrics.ChurnMetrics.SessionsDestroyed != 3 {
		t.Errorf("expected 3 sessions destroyed, got %d", metrics.ChurnMetrics.SessionsDestroyed)
	}
	if metrics.ChurnMetrics.ActiveSessions != 2 {
		t.Errorf("expected 2 active sessions, got %d", metrics.ChurnMetrics.ActiveSessions)
	}
	if metrics.ChurnMetrics.ReconnectAttempts != 1 {
		t.Errorf("expected 1 reconnect attempt, got %d", metrics.ChurnMetrics.ReconnectAttempts)
	}
}

func TestChurnMetricsMultipleSamples(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(0, 10000)

	agg.AddChurnSample(ChurnSample{
		SessionsCreated:   5,
		SessionsDestroyed: 2,
		ActiveSessions:    3,
		ReconnectAttempts: 1,
	})
	agg.AddChurnSample(ChurnSample{
		SessionsCreated:   3,
		SessionsDestroyed: 4,
		ActiveSessions:    2,
		ReconnectAttempts: 2,
	})
	agg.AddChurnSample(ChurnSample{
		SessionsCreated:   2,
		SessionsDestroyed: 1,
		ActiveSessions:    3,
		ReconnectAttempts: 0,
	})

	metrics := agg.Compute()

	if metrics.ChurnMetrics == nil {
		t.Fatal("expected churn metrics")
	}
	if metrics.ChurnMetrics.SessionsCreated != 10 {
		t.Errorf("expected 10 sessions created (5+3+2), got %d", metrics.ChurnMetrics.SessionsCreated)
	}
	if metrics.ChurnMetrics.SessionsDestroyed != 7 {
		t.Errorf("expected 7 sessions destroyed (2+4+1), got %d", metrics.ChurnMetrics.SessionsDestroyed)
	}
	if metrics.ChurnMetrics.ActiveSessions != 3 {
		t.Errorf("expected 3 active sessions (last sample), got %d", metrics.ChurnMetrics.ActiveSessions)
	}
	if metrics.ChurnMetrics.ReconnectAttempts != 3 {
		t.Errorf("expected 3 reconnect attempts (1+2+0), got %d", metrics.ChurnMetrics.ReconnectAttempts)
	}
}

func TestChurnMetricsChurnRate(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(0, 10000)

	agg.AddChurnSample(ChurnSample{
		SessionsCreated:   10,
		SessionsDestroyed: 10,
		ActiveSessions:    0,
		ReconnectAttempts: 0,
	})

	metrics := agg.Compute()

	if metrics.ChurnMetrics == nil {
		t.Fatal("expected churn metrics")
	}
	expectedRate := 2.0
	if metrics.ChurnMetrics.ChurnRate != expectedRate {
		t.Errorf("expected churn rate %.2f (20 sessions / 10 sec), got %.2f", expectedRate, metrics.ChurnMetrics.ChurnRate)
	}
}

func TestChurnMetricsChurnRateZeroDuration(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(1000, 1000)

	agg.AddChurnSample(ChurnSample{
		SessionsCreated:   5,
		SessionsDestroyed: 5,
		ActiveSessions:    0,
		ReconnectAttempts: 0,
	})

	metrics := agg.Compute()

	if metrics.ChurnMetrics == nil {
		t.Fatal("expected churn metrics")
	}
	if metrics.ChurnMetrics.ChurnRate != 0 {
		t.Errorf("expected 0 churn rate with zero duration, got %.2f", metrics.ChurnMetrics.ChurnRate)
	}
}

func TestChurnMetricsReset(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(0, 10000)

	agg.AddChurnSample(ChurnSample{
		SessionsCreated:   5,
		SessionsDestroyed: 3,
		ActiveSessions:    2,
		ReconnectAttempts: 1,
	})

	metrics := agg.Compute()
	if metrics.ChurnMetrics == nil {
		t.Fatal("expected churn metrics before reset")
	}

	agg.Reset()

	metrics = agg.Compute()
	if metrics.ChurnMetrics != nil {
		t.Error("expected nil churn metrics after reset")
	}
}

func TestChurnMetricsConcurrent(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(0, 10000)
	var wg sync.WaitGroup
	numGoroutines := 10
	samplesPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < samplesPerGoroutine; j++ {
				agg.AddChurnSample(ChurnSample{
					SessionsCreated:   1,
					SessionsDestroyed: 1,
					ActiveSessions:    id,
					ReconnectAttempts: 0,
				})
			}
		}(i)
	}

	wg.Wait()

	metrics := agg.Compute()
	if metrics.ChurnMetrics == nil {
		t.Fatal("expected churn metrics")
	}
	expectedTotal := int64(numGoroutines * samplesPerGoroutine)
	if metrics.ChurnMetrics.SessionsCreated != expectedTotal {
		t.Errorf("expected %d sessions created, got %d", expectedTotal, metrics.ChurnMetrics.SessionsCreated)
	}
	if metrics.ChurnMetrics.SessionsDestroyed != expectedTotal {
		t.Errorf("expected %d sessions destroyed, got %d", expectedTotal, metrics.ChurnMetrics.SessionsDestroyed)
	}
}

func TestChurnMetricsInAggregatedMetrics(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(0, 5000)

	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})
	agg.AddChurnSample(ChurnSample{
		SessionsCreated:   20,
		SessionsDestroyed: 15,
		ActiveSessions:    5,
		ReconnectAttempts: 3,
	})

	metrics := agg.Compute()

	if metrics.TotalOps != 1 {
		t.Errorf("expected 1 total op, got %d", metrics.TotalOps)
	}
	if metrics.ChurnMetrics == nil {
		t.Fatal("expected churn metrics in aggregated metrics")
	}
	if metrics.ChurnMetrics.SessionsCreated != 20 {
		t.Errorf("expected 20 sessions created, got %d", metrics.ChurnMetrics.SessionsCreated)
	}
	expectedRate := 7.0
	if metrics.ChurnMetrics.ChurnRate != expectedRate {
		t.Errorf("expected churn rate %.2f (35 sessions / 5 sec), got %.2f", expectedRate, metrics.ChurnMetrics.ChurnRate)
	}
}

func TestChurnMetricsHighChurnScenario(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(0, 60000)

	for i := 0; i < 100; i++ {
		agg.AddChurnSample(ChurnSample{
			SessionsCreated:   10,
			SessionsDestroyed: 9,
			ActiveSessions:    i + 1,
			ReconnectAttempts: 1,
		})
	}

	metrics := agg.Compute()

	if metrics.ChurnMetrics == nil {
		t.Fatal("expected churn metrics")
	}
	if metrics.ChurnMetrics.SessionsCreated != 1000 {
		t.Errorf("expected 1000 sessions created, got %d", metrics.ChurnMetrics.SessionsCreated)
	}
	if metrics.ChurnMetrics.SessionsDestroyed != 900 {
		t.Errorf("expected 900 sessions destroyed, got %d", metrics.ChurnMetrics.SessionsDestroyed)
	}
	if metrics.ChurnMetrics.ActiveSessions != 100 {
		t.Errorf("expected 100 active sessions (last sample), got %d", metrics.ChurnMetrics.ActiveSessions)
	}
	if metrics.ChurnMetrics.ReconnectAttempts != 100 {
		t.Errorf("expected 100 reconnect attempts, got %d", metrics.ChurnMetrics.ReconnectAttempts)
	}
	expectedRate := (1000.0 + 900.0) / 60.0
	if metrics.ChurnMetrics.ChurnRate != expectedRate {
		t.Errorf("expected churn rate %.2f, got %.2f", expectedRate, metrics.ChurnMetrics.ChurnRate)
	}
}
