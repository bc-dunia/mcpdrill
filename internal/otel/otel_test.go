package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("expected Enabled to be false by default")
	}
	if cfg.ServiceName != "mcpdrill" {
		t.Errorf("expected ServiceName 'mcpdrill', got %q", cfg.ServiceName)
	}
	if cfg.ExporterType != ExporterNone {
		t.Errorf("expected ExporterType 'none', got %q", cfg.ExporterType)
	}
	if cfg.SampleRate != 1.0 {
		t.Errorf("expected SampleRate 1.0, got %f", cfg.SampleRate)
	}
}

func TestNewTracerDisabled(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	if tracer.Enabled() {
		t.Error("expected tracer to be disabled")
	}

	spanCtx, span := tracer.StartSpan(ctx, "test-span")
	defer span.End()

	if spanCtx == nil {
		t.Error("expected non-nil context")
	}
	if span == nil {
		t.Error("expected non-nil span")
	}
}

func TestNewTracerWithNilConfig(t *testing.T) {
	ctx := context.Background()

	tracer, err := NewTracer(ctx, nil)
	if err != nil {
		t.Fatalf("NewTracer with nil config failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	if tracer.Enabled() {
		t.Error("expected tracer to be disabled with nil config")
	}
}

func TestNewTracerStdout(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer with stdout exporter failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	if !tracer.Enabled() {
		t.Error("expected tracer to be enabled")
	}
}

func TestTracerStartOperationSpan(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	opts := OperationSpanOptions{
		RunID:     "run_0000000000000123",
		StageID:   "stage-456",
		WorkerID:  "worker-789",
		VUID:      "vu-001",
		SessionID: "session-abc",
		Operation: "tools/call",
		ToolName:  "test-tool",
	}

	spanCtx, span := tracer.StartOperationSpan(ctx, opts)
	defer span.End()

	if spanCtx == nil {
		t.Error("expected non-nil context")
	}

	sc := span.SpanContext()
	if !sc.HasTraceID() {
		t.Error("expected span to have trace ID")
	}
	if !sc.HasSpanID() {
		t.Error("expected span to have span ID")
	}
}

func TestGetTraceInfo(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	spanCtx, span := tracer.StartSpan(ctx, "test-span")
	defer span.End()

	traceID, spanID := GetTraceInfo(spanCtx)

	if traceID == "" {
		t.Error("expected non-empty trace ID")
	}
	if spanID == "" {
		t.Error("expected non-empty span ID")
	}

	if len(traceID) != 32 {
		t.Errorf("expected trace ID length 32, got %d", len(traceID))
	}
	if len(spanID) != 16 {
		t.Errorf("expected span ID length 16, got %d", len(spanID))
	}
}

func TestGetTraceInfoNoSpan(t *testing.T) {
	ctx := context.Background()

	traceID, spanID := GetTraceInfo(ctx)

	if traceID != "" {
		t.Errorf("expected empty trace ID, got %q", traceID)
	}
	if spanID != "" {
		t.Errorf("expected empty span ID, got %q", spanID)
	}
}

func TestNoopTracer(t *testing.T) {
	tracer := NoopTracer()

	if tracer.Enabled() {
		t.Error("expected noop tracer to be disabled")
	}

	ctx := context.Background()
	spanCtx, span := tracer.StartSpan(ctx, "test-span")
	defer span.End()

	if spanCtx == nil {
		t.Error("expected non-nil context")
	}
}

func TestGlobalTracer(t *testing.T) {
	tracer := GetGlobalTracer()
	if tracer == nil {
		t.Error("expected non-nil global tracer")
	}
	if tracer.Enabled() {
		t.Error("expected default global tracer to be disabled")
	}

	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	newTracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer newTracer.Shutdown(ctx)

	SetGlobalTracer(newTracer)
	defer SetGlobalTracer(nil)

	globalTracer := GetGlobalTracer()
	if !globalTracer.Enabled() {
		t.Error("expected global tracer to be enabled after setting")
	}
}

func TestRecordError(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	_, span := tracer.StartSpan(ctx, "test-span")
	defer span.End()

	RecordError(span, nil, "test", false)

	testErr := errorString("test error")
	RecordError(span, testErr, "timeout", true)

	RecordError(nil, testErr, "test", false)
}

func TestRecordRetry(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	_, span := tracer.StartSpan(ctx, "test-span")
	defer span.End()

	RecordRetry(span, 1, "connection reset")
	RecordRetry(span, 2, "timeout")

	RecordRetry(nil, 1, "test")
}

func TestMiddlewareDisabled(t *testing.T) {
	tracer := NoopTracer()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := Middleware(tracer)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddlewareEnabled(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	var capturedSpan trace.Span
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSpan = trace.SpanFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := Middleware(tracer)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if capturedSpan == nil {
		t.Error("expected span to be captured in handler")
	}
}

func TestMiddlewareWithTraceparent(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	var capturedTraceID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		if span != nil {
			sc := span.SpanContext()
			if sc.HasTraceID() {
				capturedTraceID = sc.TraceID().String()
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := Middleware(tracer)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if capturedTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected trace ID from header, got %q", capturedTraceID)
	}
}

func TestMiddlewareNilTracer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := Middleware(nil)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestInjectHeaders(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	spanCtx, span := tracer.StartSpan(ctx, "test-span")
	defer span.End()

	headers := http.Header{}
	InjectHeaders(spanCtx, headers, tracer)

	traceparent := headers.Get("traceparent")
	if traceparent == "" {
		t.Error("expected traceparent header to be set")
	}
}

func TestInjectHeadersDisabled(t *testing.T) {
	ctx := context.Background()
	tracer := NoopTracer()

	headers := http.Header{}
	InjectHeaders(ctx, headers, tracer)

	traceparent := headers.Get("traceparent")
	if traceparent != "" {
		t.Errorf("expected no traceparent header, got %q", traceparent)
	}
}

func TestExtractContext(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	headers := http.Header{}
	headers.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	extractedCtx := ExtractContext(ctx, headers, tracer)

	span := trace.SpanFromContext(extractedCtx)
	if span == nil {
		t.Error("expected span from extracted context")
	}

	sc := span.SpanContext()
	if sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected trace ID from header, got %q", sc.TraceID().String())
	}
}

func TestExtractContextDisabled(t *testing.T) {
	ctx := context.Background()
	tracer := NoopTracer()

	headers := http.Header{}
	headers.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	extractedCtx := ExtractContext(ctx, headers, tracer)

	if extractedCtx != ctx {
		t.Error("expected same context when tracer is disabled")
	}
}

func TestSamplerConfigurations(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		sampleRate float64
	}{
		{"always_sample", 1.0},
		{"never_sample", 0.0},
		{"half_sample", 0.5},
		{"above_one", 1.5},
		{"below_zero", -0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Enabled:      true,
				ServiceName:  "test-service",
				ExporterType: ExporterStdout,
				SampleRate:   tt.sampleRate,
			}

			tracer, err := NewTracer(ctx, cfg)
			if err != nil {
				t.Fatalf("NewTracer failed: %v", err)
			}
			defer tracer.Shutdown(ctx)

			if !tracer.Enabled() {
				t.Error("expected tracer to be enabled")
			}
		})
	}
}

func TestTracerPropagator(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	propagator := tracer.Propagator()
	if propagator == nil {
		t.Error("expected non-nil propagator")
	}

	_, ok := propagator.(propagation.TextMapPropagator)
	if !ok {
		t.Error("expected propagator to implement TextMapPropagator")
	}
}

func TestTracerProvider(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	tp := tracer.TracerProvider()
	if tp == nil {
		t.Error("expected non-nil tracer provider")
	}
}

func TestSpanFromContext(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:      true,
		ServiceName:  "test-service",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	spanCtx, span := tracer.StartSpan(ctx, "test-span")
	defer span.End()

	retrievedSpan := tracer.SpanFromContext(spanCtx)
	if retrievedSpan == nil {
		t.Error("expected non-nil span from context")
	}

	if retrievedSpan.SpanContext().SpanID() != span.SpanContext().SpanID() {
		t.Error("expected same span from context")
	}
}

func TestConfigWithAttributes(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:        true,
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		ExporterType:   ExporterStdout,
		SampleRate:     1.0,
		Attributes: map[string]string{
			"environment": "test",
			"region":      "us-west-2",
		},
	}

	tracer, err := NewTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Shutdown(ctx)

	if !tracer.Enabled() {
		t.Error("expected tracer to be enabled")
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
