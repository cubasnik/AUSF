package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type traceIDKeyType string

const (
	traceHeaderName      = "X-Trace-Id"
	traceparentHeader    = "traceparent"
	oTelTraceparentFlags = "01"
)

const unknownRoute = "unknown"

var traceIDKey traceIDKeyType = "trace-id"

var (
	metricsCollector = &httpMetrics{
		requests: make(map[string]float64),
		sum:      make(map[string]float64),
		count:    make(map[string]float64),
	}
)

func withObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		traceID := extractOrCreateTraceID(request)
		traceparent := formatTraceparent(traceID)

		ctx := context.WithValue(request.Context(), traceIDKey, traceID)
		request = request.WithContext(ctx)
		request.Header.Set(traceparentHeader, traceparent)

		writer.Header().Set(traceHeaderName, traceID)
		writer.Header().Set(traceparentHeader, traceparent)

		route := normalizeRoute(request.URL.Path)
		start := time.Now()
		wrapped := &statusRecorder{ResponseWriter: writer, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, request)
		duration := time.Since(start)

		metricsCollector.observe(request.Method, route, wrapped.statusCode, duration.Seconds())

		log.Printf(
			"trace_id=%s method=%s route=%s status=%d duration_ms=%.2f remote=%s",
			traceID,
			request.Method,
			route,
			wrapped.statusCode,
			float64(duration.Microseconds())/1000.0,
			request.RemoteAddr,
		)
	})
}

func (handler Handler) metrics(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = writer.Write([]byte(metricsCollector.prometheusText()))
}

func TraceIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(traceIDKey).(string); ok {
		return value
	}
	return ""
}

func extractOrCreateTraceID(request *http.Request) string {
	if traceparent := strings.TrimSpace(request.Header.Get(traceparentHeader)); traceparent != "" {
		if traceID, ok := traceIDFromTraceparent(traceparent); ok {
			return traceID
		}
	}

	if traceID := strings.TrimSpace(request.Header.Get(traceHeaderName)); traceID != "" {
		if isHexTraceID(traceID) {
			return strings.ToLower(traceID)
		}
	}

	return newTraceID()
}

func traceIDFromTraceparent(traceparent string) (string, bool) {
	parts := strings.Split(strings.TrimSpace(traceparent), "-")
	if len(parts) != 4 {
		return "", false
	}

	traceID := strings.ToLower(parts[1])
	if len(traceID) != 32 || !isHexTraceID(traceID) {
		return "", false
	}

	return traceID, true
}

func isHexTraceID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func newTraceID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(bytes)
}

func formatTraceparent(traceID string) string {
	const spanID = "0000000000000001"
	return "00-" + traceID + "-" + spanID + "-" + oTelTraceparentFlags
}

func normalizeRoute(path string) string {
	switch {
	case path == "/healthz":
		return "/healthz"
	case path == "/metrics":
		return "/metrics"
	case path == "/nausf-auth/v1/ue-authentications":
		return "/nausf-auth/v1/ue-authentications"
	case strings.HasPrefix(path, "/nausf-auth/v1/ue-authentications/"):
		trimmed := strings.TrimPrefix(path, "/nausf-auth/v1/ue-authentications/")
		parts := strings.Split(strings.Trim(trimmed, "/"), "/")
		if len(parts) == 1 {
			return "/nausf-auth/v1/ue-authentications/{authCtxId}"
		}
		if len(parts) == 2 {
			switch parts[1] {
			case "5g-aka-confirmation":
				return "/nausf-auth/v1/ue-authentications/{authCtxId}/5g-aka-confirmation"
			case "eap-session":
				return "/nausf-auth/v1/ue-authentications/{authCtxId}/eap-session"
			}
		}
		return "/nausf-auth/v1/ue-authentications/*"
	default:
		return unknownRoute
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (recorder *statusRecorder) WriteHeader(statusCode int) {
	recorder.statusCode = statusCode
	recorder.ResponseWriter.WriteHeader(statusCode)
}

type httpMetrics struct {
	mu       sync.RWMutex
	requests map[string]float64
	sum      map[string]float64
	count    map[string]float64
}

func (metrics *httpMetrics) observe(method string, route string, statusCode int, durationSeconds float64) {
	status := strconv.Itoa(statusCode)

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	requestKey := metricLabels(method, route, status)
	metrics.requests[requestKey]++

	durationKey := metricLabels(method, route, "")
	metrics.sum[durationKey] += durationSeconds
	metrics.count[durationKey]++
}

func (metrics *httpMetrics) prometheusText() string {
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()

	var builder strings.Builder
	builder.WriteString("# HELP ausf_http_requests_total Total number of HTTP requests handled by AUSF.\n")
	builder.WriteString("# TYPE ausf_http_requests_total counter\n")
	for labels, value := range metrics.requests {
		builder.WriteString(fmt.Sprintf("ausf_http_requests_total{%s} %g\n", labels, value))
	}

	builder.WriteString("# HELP ausf_http_request_duration_seconds Request duration metrics for AUSF handlers.\n")
	builder.WriteString("# TYPE ausf_http_request_duration_seconds summary\n")
	for labels, value := range metrics.sum {
		builder.WriteString(fmt.Sprintf("ausf_http_request_duration_seconds_sum{%s} %g\n", labels, value))
	}
	for labels, value := range metrics.count {
		builder.WriteString(fmt.Sprintf("ausf_http_request_duration_seconds_count{%s} %g\n", labels, value))
	}

	return builder.String()
}

func metricLabels(method string, route string, status string) string {
	if status == "" {
		return fmt.Sprintf("method=\"%s\",route=\"%s\"", method, route)
	}
	return fmt.Sprintf("method=\"%s\",route=\"%s\",status=\"%s\"", method, route, status)
}
