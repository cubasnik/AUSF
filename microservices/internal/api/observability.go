package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alexey/ausf/microservices/internal/metrics"
)

type traceIDKeyType string

const (
	traceHeaderName      = "X-Trace-Id"
	traceparentHeader    = "traceparent"
	oTelTraceparentFlags = "01"
)

const unknownRoute = "unknown"

var traceIDKey traceIDKeyType = "trace-id"

var defaultRegistry = metrics.NewRegistry()

// SetDefaultRegistry replaces the registry used by the observability middleware
// and the /metrics handler. Call once at startup before serving requests.
func SetDefaultRegistry(r *metrics.Registry) {
	defaultRegistry = r
}

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

		defaultRegistry.ObserveHTTP(request.Method, route, wrapped.statusCode, duration.Seconds())

		logJSON(map[string]any{
			"level":       "INFO",
			"msg":         "request",
			"trace_id":    traceID,
			"method":      request.Method,
			"route":       route,
			"status":      wrapped.statusCode,
			"duration_ms": float64(duration.Microseconds()) / 1000.0,
			"remote":      request.RemoteAddr,
		})
	})
}

func (handler Handler) metrics(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = writer.Write([]byte(defaultRegistry.PrometheusText()))
}

func logJSON(fields map[string]any) {
	fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	data, _ := json.Marshal(fields)
	_, _ = fmt.Fprintln(os.Stderr, string(data))
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
