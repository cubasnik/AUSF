package tracing

import (
	"context"
	"strings"
	"testing"
)

func TestSpanContextIsValid(t *testing.T) {
	sc := SpanContext{TraceID: strings.Repeat("a", 32), SpanID: strings.Repeat("b", 16)}
	if !sc.IsValid() {
		t.Fatal("IsValid() = false, want true")
	}
}

func TestSpanContextIsInvalidWhenEmpty(t *testing.T) {
	var sc SpanContext
	if sc.IsValid() {
		t.Fatal("IsValid() = true, want false for zero SpanContext")
	}
}

func TestTraceparentFormat(t *testing.T) {
	sc := SpanContext{TraceID: strings.Repeat("a", 32), SpanID: strings.Repeat("b", 16)}
	tp := sc.Traceparent()
	if !strings.HasPrefix(tp, "00-") {
		t.Fatalf("Traceparent() = %q, want prefix 00-", tp)
	}
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		t.Fatalf("Traceparent() has %d parts, want 4", len(parts))
	}
}

func TestStartSpanCreatesChildSpan(t *testing.T) {
	exporter := NewExporter("", "test")
	tracer := NewTracer("test", exporter)

	ctx := context.Background()
	ctx, parent := tracer.StartSpan(ctx, "parent", SpanKindServer, "", "")
	if !parent.SpanContext().IsValid() {
		t.Fatal("parent span context is invalid")
	}

	_, child := tracer.StartSpan(ctx, "child", SpanKindClient, parent.SpanContext().TraceID, parent.SpanContext().SpanID)
	if child.SpanContext().TraceID != parent.SpanContext().TraceID {
		t.Fatalf("child trace ID %s != parent trace ID %s", child.SpanContext().TraceID, parent.SpanContext().TraceID)
	}
	if child.SpanContext().ParentID != parent.SpanContext().SpanID {
		t.Fatalf("child parent ID %s != parent span ID %s", child.SpanContext().ParentID, parent.SpanContext().SpanID)
	}
}

func TestContextRoundTrip(t *testing.T) {
	sc := SpanContext{TraceID: strings.Repeat("c", 32), SpanID: strings.Repeat("d", 16)}
	ctx := ContextWithSpanContext(context.Background(), sc)
	got := SpanContextFromContext(ctx)
	if got.TraceID != sc.TraceID || got.SpanID != sc.SpanID {
		t.Fatalf("round-trip failed: got %+v, want %+v", got, sc)
	}
}

func TestSpanEndWithNoopExporterDoesNotPanic(t *testing.T) {
	exporter := NewExporter("", "test") // no-op
	tracer := NewTracer("test", exporter)
	_, span := tracer.StartSpan(context.Background(), "op", SpanKindInternal, "", "")
	span.SetAttribute("key", "value")
	span.SetStatus(StatusOK, "")
	span.End() // should not panic
}
