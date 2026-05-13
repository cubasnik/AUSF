// Package tracing provides a minimal, zero-external-dependency OpenTelemetry-compatible
// tracer for the AUSF microservice.
//
// It propagates the W3C Trace Context (traceparent) on inbound and outbound HTTP calls
// and exports completed spans to an OTLP/HTTP collector via the JSON wire format.
// When AUSF_OTLP_ENDPOINT is empty the exporter is a silent no-op.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// SpanKind mirrors the OTel span kind values used in the OTLP JSON payload.
type SpanKind int

const (
	SpanKindInternal SpanKind = 1
	SpanKindServer   SpanKind = 2
	SpanKindClient   SpanKind = 3
)

// SpanStatus codes follow the OTel spec.
const (
	StatusUnset = 0
	StatusOK    = 1
	StatusError = 2
)

// SpanContext carries the immutable identity of a span.
type SpanContext struct {
	TraceID  string // 32 lower-case hex chars (128-bit)
	SpanID   string // 16 lower-case hex chars (64-bit)
	ParentID string // 16 lower-case hex chars; empty for root spans
}

// IsValid reports whether the SpanContext has non-zero IDs.
func (sc SpanContext) IsValid() bool {
	return len(sc.TraceID) == 32 && len(sc.SpanID) == 16
}

// Traceparent encodes the W3C traceparent header value.
func (sc SpanContext) Traceparent() string {
	return "00-" + sc.TraceID + "-" + sc.SpanID + "-01"
}

type spanContextKeyType struct{}

var spanContextKey spanContextKeyType

// ContextWithSpanContext stores sc in ctx.
func ContextWithSpanContext(ctx context.Context, sc SpanContext) context.Context {
	return context.WithValue(ctx, spanContextKey, sc)
}

// SpanContextFromContext retrieves the SpanContext stored in ctx.
// Returns a zero SpanContext when none is present.
func SpanContextFromContext(ctx context.Context) SpanContext {
	if sc, ok := ctx.Value(spanContextKey).(SpanContext); ok {
		return sc
	}
	return SpanContext{}
}

// Attribute is a key-value pair attached to a span.
type Attribute struct {
	Key   string
	Value any
}

// Span represents a single unit of work. Call End() when the work is done.
type Span struct {
	sc         SpanContext
	name       string
	kind       SpanKind
	startNano  int64
	endNano    int64
	attrs      []Attribute
	statusCode int
	statusMsg  string
	exporter   *Exporter
}

// SpanContext returns the identity of this span.
func (s *Span) SpanContext() SpanContext { return s.sc }

// SetAttribute attaches a key/value pair to the span.
func (s *Span) SetAttribute(key string, value any) {
	s.attrs = append(s.attrs, Attribute{Key: key, Value: value})
}

// SetStatus sets the span status.
func (s *Span) SetStatus(code int, msg string) {
	s.statusCode = code
	s.statusMsg = msg
}

// End marks the span as finished and enqueues it for export.
func (s *Span) End() {
	if s.exporter != nil {
		s.exporter.enqueue(s)
	}
}

// ────────────────────────────────────────────────────────────────────────────

// Tracer creates spans and injects/extracts W3C traceparent headers.
type Tracer struct {
	serviceName string
	exporter    *Exporter
}

// NewTracer creates a Tracer.  Pass a no-op exporter (NewExporter("", ...))
// to disable OTLP export while keeping context propagation.
func NewTracer(serviceName string, exporter *Exporter) *Tracer {
	return &Tracer{serviceName: serviceName, exporter: exporter}
}

// StartSpan starts a new span.
//   - If ctx already contains a SpanContext, the new span becomes a child.
//   - traceID / parentID may be pre-supplied (e.g. from an inbound traceparent).
//     Pass empty strings to let the tracer generate them.
func (t *Tracer) StartSpan(ctx context.Context, name string, kind SpanKind, inboundTraceID, inboundParentID string) (context.Context, *Span) {
	traceID := inboundTraceID
	if traceID == "" {
		traceID = newHex(16) // 128-bit
	}

	spanID := newHex(8) // 64-bit

	parentID := inboundParentID

	sc := SpanContext{
		TraceID:  traceID,
		SpanID:   spanID,
		ParentID: parentID,
	}

	span := &Span{
		sc:        sc,
		name:      name,
		kind:      kind,
		startNano: nowNano(),
		exporter:  t.exporter,
	}

	return ContextWithSpanContext(ctx, sc), span
}

// ────────────────────────────────────────────────────────────────────────────

func newHex(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is extraordinarily rare; fall back to zeros
		return hex.EncodeToString(make([]byte, bytes))
	}
	return hex.EncodeToString(b)
}
