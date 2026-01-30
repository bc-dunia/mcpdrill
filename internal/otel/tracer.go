// Package otel provides OpenTelemetry tracing integration for mcpdrill.
package otel

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// ExporterType defines the type of trace exporter to use.
type ExporterType string

const (
	// ExporterNone disables tracing (no-op).
	ExporterNone ExporterType = "none"
	// ExporterStdout exports traces to stdout (useful for debugging).
	ExporterStdout ExporterType = "stdout"
	// ExporterOTLPGRPC exports traces via OTLP over gRPC.
	ExporterOTLPGRPC ExporterType = "otlp-grpc"
	// ExporterOTLPHTTP exports traces via OTLP over HTTP.
	ExporterOTLPHTTP ExporterType = "otlp-http"
)

// Config holds configuration for the OpenTelemetry tracer.
type Config struct {
	// Enabled controls whether tracing is active. Default: false (no-op).
	Enabled bool

	// ServiceName is the name of the service for trace attribution.
	ServiceName string

	// ServiceVersion is the version of the service.
	ServiceVersion string

	// ExporterType specifies which exporter to use.
	ExporterType ExporterType

	// OTLPEndpoint is the endpoint for OTLP exporters (e.g., "localhost:4317").
	OTLPEndpoint string

	// OTLPInsecure disables TLS for OTLP connections.
	OTLPInsecure bool

	// SampleRate is the sampling rate (0.0 to 1.0). Default: 1.0 (sample all).
	SampleRate float64

	// Attributes are additional attributes to add to all spans.
	Attributes map[string]string
}

// DefaultConfig returns a default configuration with tracing disabled.
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		ServiceName:  "mcpdrill",
		ExporterType: ExporterNone,
		SampleRate:   1.0,
	}
}

// Tracer wraps OpenTelemetry tracing functionality with mcpdrill-specific helpers.
type Tracer struct {
	config         *Config
	tracerProvider trace.TracerProvider
	tracer         trace.Tracer
	propagator     propagation.TextMapPropagator
	shutdown       func(context.Context) error
	mu             sync.RWMutex
}

// globalTracer is the singleton tracer instance.
var (
	globalTracer *Tracer
	globalMu     sync.RWMutex
)

// NewTracer creates a new Tracer with the given configuration.
func NewTracer(ctx context.Context, cfg *Config) (*Tracer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	t := &Tracer{
		config:     cfg,
		propagator: propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	}

	if !cfg.Enabled || cfg.ExporterType == ExporterNone {
		// Use no-op tracer when disabled
		t.tracerProvider = noop.NewTracerProvider()
		t.tracer = t.tracerProvider.Tracer(cfg.ServiceName)
		t.shutdown = func(context.Context) error { return nil }
		return t, nil
	}

	// Create exporter based on type
	exporter, err := t.createExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create resource with service information
	res, err := t.createResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	t.tracerProvider = tp
	t.tracer = tp.Tracer(cfg.ServiceName)
	t.shutdown = tp.Shutdown

	// Set global propagator
	otel.SetTextMapPropagator(t.propagator)

	return t, nil
}

// createExporter creates the appropriate exporter based on configuration.
func (t *Tracer) createExporter(ctx context.Context, cfg *Config) (sdktrace.SpanExporter, error) {
	switch cfg.ExporterType {
	case ExporterStdout:
		return stdouttrace.New(stdouttrace.WithPrettyPrint())

	case ExporterOTLPGRPC:
		opts := []otlptracegrpc.Option{}
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx, opts...)

	case ExporterOTLPHTTP:
		opts := []otlptracehttp.Option{}
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, opts...)

	default:
		return nil, fmt.Errorf("unknown exporter type: %s", cfg.ExporterType)
	}
}

// createResource creates the OpenTelemetry resource with service information.
func (t *Tracer) createResource(cfg *Config) (*resource.Resource, error) {
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

// Shutdown gracefully shuts down the tracer, flushing any pending spans.
func (t *Tracer) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.shutdown != nil {
		return t.shutdown(ctx)
	}
	return nil
}

// Enabled returns whether tracing is enabled.
func (t *Tracer) Enabled() bool {
	return t.config.Enabled && t.config.ExporterType != ExporterNone
}

// StartSpan starts a new span with the given name and options.
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the current span from the context.
func (t *Tracer) SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// Propagator returns the text map propagator for context propagation.
func (t *Tracer) Propagator() propagation.TextMapPropagator {
	return t.propagator
}

// TracerProvider returns the underlying tracer provider.
func (t *Tracer) TracerProvider() trace.TracerProvider {
	return t.tracerProvider
}

// OperationSpanOptions contains options for starting an operation span.
type OperationSpanOptions struct {
	RunID     string
	StageID   string
	WorkerID  string
	VUID      string
	SessionID string
	Operation string
	ToolName  string
}

// StartOperationSpan starts a span for an MCP operation with standard attributes.
func (t *Tracer) StartOperationSpan(ctx context.Context, opts OperationSpanOptions) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("mcpdrill.run_id", opts.RunID),
		attribute.String("mcpdrill.stage_id", opts.StageID),
		attribute.String("mcpdrill.worker_id", opts.WorkerID),
		attribute.String("mcpdrill.vu_id", opts.VUID),
		attribute.String("mcpdrill.operation", opts.Operation),
	}

	if opts.SessionID != "" {
		attrs = append(attrs, attribute.String("mcpdrill.session_id", opts.SessionID))
	}

	if opts.ToolName != "" {
		attrs = append(attrs, attribute.String("mcpdrill.tool_name", opts.ToolName))
	}

	spanName := fmt.Sprintf("mcp.%s", opts.Operation)
	if opts.ToolName != "" {
		spanName = fmt.Sprintf("mcp.%s/%s", opts.Operation, opts.ToolName)
	}

	return t.tracer.Start(ctx, spanName,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// RecordError records an error on the span with additional context.
func RecordError(span trace.Span, err error, errorType string, retryable bool) {
	if span == nil || err == nil {
		return
	}

	span.RecordError(err)
	span.SetAttributes(
		attribute.String("error.type", errorType),
		attribute.Bool("error.retryable", retryable),
	)
}

// RecordRetry records a retry event on the span.
func RecordRetry(span trace.Span, attempt int, reason string) {
	if span == nil {
		return
	}

	span.AddEvent("retry",
		trace.WithAttributes(
			attribute.Int("retry.attempt", attempt),
			attribute.String("retry.reason", reason),
		),
	)
}

// GetTraceInfo extracts trace ID and span ID from the current span.
func GetTraceInfo(ctx context.Context) (traceID, spanID string) {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return "", ""
	}

	sc := span.SpanContext()
	if sc.HasTraceID() {
		traceID = sc.TraceID().String()
	}
	if sc.HasSpanID() {
		spanID = sc.SpanID().String()
	}

	return traceID, spanID
}

// SetGlobalTracer sets the global tracer instance.
func SetGlobalTracer(t *Tracer) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalTracer = t

	if t != nil && t.Enabled() {
		otel.SetTracerProvider(t.tracerProvider)
	}
}

// GetGlobalTracer returns the global tracer instance.
// Returns a no-op tracer if none has been set.
func GetGlobalTracer() *Tracer {
	globalMu.RLock()
	defer globalMu.RUnlock()

	if globalTracer == nil {
		// Return a no-op tracer
		return &Tracer{
			config:         DefaultConfig(),
			tracerProvider: noop.NewTracerProvider(),
			tracer:         noop.NewTracerProvider().Tracer("mcpdrill"),
			propagator:     propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}),
			shutdown:       func(context.Context) error { return nil },
		}
	}

	return globalTracer
}

// NoopTracer returns a tracer that does nothing (for testing or when disabled).
func NoopTracer() *Tracer {
	tp := noop.NewTracerProvider()
	return &Tracer{
		config:         DefaultConfig(),
		tracerProvider: tp,
		tracer:         tp.Tracer("mcpdrill"),
		propagator:     propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}),
		shutdown:       func(context.Context) error { return nil },
	}
}
