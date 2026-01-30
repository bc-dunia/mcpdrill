// Package otel provides OpenTelemetry metrics integration for mcpdrill.
package otel

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// MetricsConfig holds configuration for the OpenTelemetry metrics.
type MetricsConfig struct {
	// Enabled controls whether metrics collection is active. Default: false (no-op).
	Enabled bool

	// ServiceName is the name of the service for metric attribution.
	ServiceName string

	// ServiceVersion is the version of the service.
	ServiceVersion string

	// ExporterType specifies which exporter to use.
	ExporterType ExporterType

	// OTLPEndpoint is the endpoint for OTLP exporters (e.g., "localhost:4317").
	OTLPEndpoint string

	// OTLPInsecure disables TLS for OTLP connections.
	OTLPInsecure bool

	// Attributes are additional attributes to add to all metrics.
	Attributes map[string]string
}

// DefaultMetricsConfig returns a default configuration with metrics disabled.
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		Enabled:      false,
		ServiceName:  "mcpdrill",
		ExporterType: ExporterNone,
	}
}

// Metrics wraps OpenTelemetry metrics functionality with mcpdrill-specific helpers.
type Metrics struct {
	config           *MetricsConfig
	meterProvider    *sdkmetric.MeterProvider
	meter            metric.Meter
	shutdown         func(context.Context) error
	mu               sync.RWMutex
	currentStage     atomic.Int64
	stageCallback    metric.Int64ObservableGauge
	stageCallbackReg metric.Registration

	// Metric instruments
	operationLatency metric.Float64Histogram
	errorCounter     metric.Int64Counter
	activeSessions   metric.Int64UpDownCounter
	reconnectCounter metric.Int64Counter
	stallCounter     metric.Int64Counter
}

// globalMetrics is the singleton metrics instance.
var (
	globalMetrics   *Metrics
	globalMetricsMu sync.RWMutex
)

// NewMetrics creates a new Metrics instance with the given configuration.
func NewMetrics(ctx context.Context, cfg *MetricsConfig) (*Metrics, error) {
	if cfg == nil {
		cfg = DefaultMetricsConfig()
	}

	m := &Metrics{
		config: cfg,
	}

	if !cfg.Enabled || cfg.ExporterType == ExporterNone {
		// Use no-op meter when disabled
		m.meterProvider = sdkmetric.NewMeterProvider()
		m.meter = m.meterProvider.Meter(cfg.ServiceName)
		m.shutdown = func(context.Context) error { return nil }
		return m, nil
	}

	// Create exporter based on type
	exporter, err := m.createExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics exporter: %w", err)
	}

	// Create resource with service information
	res, err := m.createResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics resource: %w", err)
	}

	// Create meter provider
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)

	m.meterProvider = mp
	m.meter = mp.Meter(cfg.ServiceName)
	m.shutdown = mp.Shutdown

	// Register metric instruments
	if err := m.registerInstruments(); err != nil {
		return nil, fmt.Errorf("failed to register metric instruments: %w", err)
	}

	return m, nil
}

// createExporter creates the appropriate metrics exporter based on configuration.
func (m *Metrics) createExporter(ctx context.Context, cfg *MetricsConfig) (sdkmetric.Exporter, error) {
	switch cfg.ExporterType {
	case ExporterStdout:
		return stdoutmetric.New()

	case ExporterOTLPGRPC:
		opts := []otlpmetricgrpc.Option{}
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}
		return otlpmetricgrpc.New(ctx, opts...)

	case ExporterOTLPHTTP:
		opts := []otlpmetrichttp.Option{}
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		return otlpmetrichttp.New(ctx, opts...)

	default:
		return nil, fmt.Errorf("unknown exporter type: %s", cfg.ExporterType)
	}
}

// createResource creates the OpenTelemetry resource with service information.
func (m *Metrics) createResource(cfg *MetricsConfig) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
	}

	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}

	// Add custom attributes
	for k, v := range cfg.Attributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes("", attrs...),
	)
}

// registerInstruments creates and registers all metric instruments.
func (m *Metrics) registerInstruments() error {
	var err error

	// Operation latency histogram (in milliseconds)
	m.operationLatency, err = m.meter.Float64Histogram(
		"mcpdrill.operation.latency",
		metric.WithDescription("Latency of MCP operations"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return fmt.Errorf("failed to create operation latency histogram: %w", err)
	}

	// Error counter with category attribute
	m.errorCounter, err = m.meter.Int64Counter(
		"mcpdrill.errors",
		metric.WithDescription("Count of errors by category"),
	)
	if err != nil {
		return fmt.Errorf("failed to create error counter: %w", err)
	}

	// Active sessions gauge (up/down counter)
	m.activeSessions, err = m.meter.Int64UpDownCounter(
		"mcpdrill.sessions.active",
		metric.WithDescription("Number of active MCP sessions"),
	)
	if err != nil {
		return fmt.Errorf("failed to create active sessions counter: %w", err)
	}

	// Reconnect counter
	m.reconnectCounter, err = m.meter.Int64Counter(
		"mcpdrill.reconnects",
		metric.WithDescription("Count of session reconnections"),
	)
	if err != nil {
		return fmt.Errorf("failed to create reconnect counter: %w", err)
	}

	// Stall counter
	m.stallCounter, err = m.meter.Int64Counter(
		"mcpdrill.stalls",
		metric.WithDescription("Count of stream stalls"),
	)
	if err != nil {
		return fmt.Errorf("failed to create stall counter: %w", err)
	}

	// Current stage observable gauge
	m.stageCallback, err = m.meter.Int64ObservableGauge(
		"mcpdrill.stage",
		metric.WithDescription("Current test stage index"),
	)
	if err != nil {
		return fmt.Errorf("failed to create stage gauge: %w", err)
	}

	// Register callback for stage gauge
	m.stageCallbackReg, err = m.meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			o.ObserveInt64(m.stageCallback, m.currentStage.Load())
			return nil
		},
		m.stageCallback,
	)
	if err != nil {
		return fmt.Errorf("failed to register stage gauge callback: %w", err)
	}

	return nil
}

// RecordOperationLatency records the latency of an MCP operation.
func (m *Metrics) RecordOperationLatency(ctx context.Context, operation, toolName string, latencyMs float64, success bool) {
	if m.operationLatency == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
		attribute.Bool("success", success),
	}

	if toolName != "" {
		attrs = append(attrs, attribute.String("tool_name", toolName))
	}

	m.operationLatency.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}

// RecordError records an error with the specified category.
func (m *Metrics) RecordError(ctx context.Context, category string) {
	if m.errorCounter == nil {
		return
	}

	m.errorCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("category", category),
	))
}

// IncrementSessions increments the active sessions counter.
func (m *Metrics) IncrementSessions(ctx context.Context) {
	if m.activeSessions == nil {
		return
	}

	m.activeSessions.Add(ctx, 1)
}

// DecrementSessions decrements the active sessions counter.
func (m *Metrics) DecrementSessions(ctx context.Context) {
	if m.activeSessions == nil {
		return
	}

	m.activeSessions.Add(ctx, -1)
}

// RecordReconnect increments the reconnect counter.
func (m *Metrics) RecordReconnect(ctx context.Context) {
	if m.reconnectCounter == nil {
		return
	}

	m.reconnectCounter.Add(ctx, 1)
}

// RecordStall increments the stall counter.
func (m *Metrics) RecordStall(ctx context.Context) {
	if m.stallCounter == nil {
		return
	}

	m.stallCounter.Add(ctx, 1)
}

// SetCurrentStage sets the current stage index for the observable gauge.
// This is thread-safe and will be read by the gauge callback.
func (m *Metrics) SetCurrentStage(stageIndex int) {
	m.currentStage.Store(int64(stageIndex))
}

// Shutdown gracefully shuts down the metrics provider, flushing any pending metrics.
func (m *Metrics) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Unregister callback if registered
	if m.stageCallbackReg != nil {
		if err := m.stageCallbackReg.Unregister(); err != nil {
			return fmt.Errorf("failed to unregister stage callback: %w", err)
		}
	}

	if m.shutdown != nil {
		return m.shutdown(ctx)
	}
	return nil
}

// Enabled returns whether metrics collection is enabled.
func (m *Metrics) Enabled() bool {
	return m.config.Enabled && m.config.ExporterType != ExporterNone
}

// MeterProvider returns the underlying meter provider.
func (m *Metrics) MeterProvider() *sdkmetric.MeterProvider {
	return m.meterProvider
}

// SetGlobalMetrics sets the global metrics instance.
func SetGlobalMetrics(m *Metrics) {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()
	globalMetrics = m

	if m != nil && m.Enabled() {
		otel.SetMeterProvider(m.meterProvider)
	}
}

// GetGlobalMetrics returns the global metrics instance.
// Returns a no-op metrics instance if none has been set.
func GetGlobalMetrics() *Metrics {
	globalMetricsMu.RLock()
	defer globalMetricsMu.RUnlock()

	if globalMetrics == nil {
		// Return a no-op metrics instance
		cfg := DefaultMetricsConfig()
		m := &Metrics{
			config:        cfg,
			meterProvider: sdkmetric.NewMeterProvider(),
			shutdown:      func(context.Context) error { return nil },
		}
		m.meter = m.meterProvider.Meter(cfg.ServiceName)
		return m
	}

	return globalMetrics
}

// NoopMetrics returns a metrics instance that does nothing (for testing or when disabled).
func NoopMetrics() *Metrics {
	cfg := DefaultMetricsConfig()
	mp := sdkmetric.NewMeterProvider()
	return &Metrics{
		config:        cfg,
		meterProvider: mp,
		meter:         mp.Meter(cfg.ServiceName),
		shutdown:      func(context.Context) error { return nil },
	}
}
