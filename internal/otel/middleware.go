package otel

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Middleware returns an HTTP middleware that extracts/injects W3C traceparent headers.
func Middleware(tracer *Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tracer == nil || !tracer.Enabled() {
				next.ServeHTTP(w, r)
				return
			}

			ctx := tracer.Propagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			spanName := r.Method + " " + r.URL.Path
			ctx, span := tracer.StartSpan(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.URLPath(r.URL.Path),
					semconv.URLScheme(r.URL.Scheme),
					attribute.String("http.host", r.Host),
				),
			)
			defer span.End()

			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r.WithContext(ctx))

			span.SetAttributes(semconv.HTTPResponseStatusCode(rw.statusCode))

			if rw.statusCode >= 400 {
				span.SetAttributes(attribute.Bool("error", true))
			}
		})
	}
}

// InjectHeaders injects trace context into outgoing HTTP headers.
func InjectHeaders(ctx context.Context, headers http.Header, tracer *Tracer) {
	if tracer == nil || !tracer.Enabled() {
		return
	}
	tracer.Propagator().Inject(ctx, propagation.HeaderCarrier(headers))
}

// ExtractContext extracts trace context from incoming HTTP headers.
func ExtractContext(ctx context.Context, headers http.Header, tracer *Tracer) context.Context {
	if tracer == nil || !tracer.Enabled() {
		return ctx
	}
	return tracer.Propagator().Extract(ctx, propagation.HeaderCarrier(headers))
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}
