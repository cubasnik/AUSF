// Package metrics provides a thread-safe Prometheus text format metrics registry
// for HTTP and domain authentication events in AUSF.
package metrics

import (
	"fmt"
	"strings"
	"sync"
)

// AuthRecorder records domain authentication events.
// Implemented by [Registry]; can be stubbed in tests.
type AuthRecorder interface {
	RecordAuthInitiated(authType string)
	RecordAuthConfirmed(authType string)
	RecordAuthFailed(cause string)
}

// Registry collects HTTP and auth-domain metrics and renders them in Prometheus text format.
type Registry struct {
	mu            sync.RWMutex
	httpRequests  map[string]float64
	httpSum       map[string]float64
	httpCount     map[string]float64
	authInitiated map[string]float64
	authConfirmed map[string]float64
	authFailed    map[string]float64
}

// NewRegistry returns an initialised Registry.
func NewRegistry() *Registry {
	return &Registry{
		httpRequests:  make(map[string]float64),
		httpSum:       make(map[string]float64),
		httpCount:     make(map[string]float64),
		authInitiated: make(map[string]float64),
		authConfirmed: make(map[string]float64),
		authFailed:    make(map[string]float64),
	}
}

// ObserveHTTP records a single HTTP request.
func (r *Registry) ObserveHTTP(method, route string, statusCode int, durationSeconds float64) {
	status := fmt.Sprintf("%d", statusCode)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.httpRequests[httpLabel(method, route, status)]++
	key := httpLabel(method, route, "")
	r.httpSum[key] += durationSeconds
	r.httpCount[key]++
}

// RecordAuthInitiated increments the counter for initiated authentication sessions.
func (r *Registry) RecordAuthInitiated(authType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authInitiated[fmt.Sprintf(`auth_type="%s"`, authType)]++
}

// RecordAuthConfirmed increments the counter for confirmed (successful) authentication sessions.
func (r *Registry) RecordAuthConfirmed(authType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authConfirmed[fmt.Sprintf(`auth_type="%s"`, authType)]++
}

// RecordAuthFailed increments the counter for failed authentication attempts.
func (r *Registry) RecordAuthFailed(cause string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authFailed[fmt.Sprintf(`cause="%s"`, cause)]++
}

// PrometheusText renders all collected metrics in Prometheus text exposition format.
func (r *Registry) PrometheusText() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var b strings.Builder
	writeCounter(&b, "ausf_http_requests_total", "Total HTTP requests handled by AUSF.", r.httpRequests)
	writeSummary(&b, "ausf_http_request_duration_seconds", "Request duration for AUSF handlers.", r.httpSum, r.httpCount)
	writeCounter(&b, "ausf_auth_initiated_total", "Total authentication sessions initiated by type.", r.authInitiated)
	writeCounter(&b, "ausf_auth_confirmed_total", "Total authentication sessions confirmed successfully by type.", r.authConfirmed)
	writeCounter(&b, "ausf_auth_failed_total", "Total authentication failures by cause.", r.authFailed)
	return b.String()
}

func writeCounter(b *strings.Builder, name, help string, m map[string]float64) {
	b.WriteString("# HELP " + name + " " + help + "\n")
	b.WriteString("# TYPE " + name + " counter\n")
	for labels, val := range m {
		fmt.Fprintf(b, "%s{%s} %g\n", name, labels, val)
	}
}

func writeSummary(b *strings.Builder, name, help string, sumMap, countMap map[string]float64) {
	b.WriteString("# HELP " + name + " " + help + "\n")
	b.WriteString("# TYPE " + name + " summary\n")
	for labels, val := range sumMap {
		fmt.Fprintf(b, "%s_sum{%s} %g\n", name, labels, val)
	}
	for labels, val := range countMap {
		fmt.Fprintf(b, "%s_count{%s} %g\n", name, labels, val)
	}
}

func httpLabel(method, route, status string) string {
	if status == "" {
		return fmt.Sprintf(`method="%s",route="%s"`, method, route)
	}
	return fmt.Sprintf(`method="%s",route="%s",status="%s"`, method, route, status)
}
