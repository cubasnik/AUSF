// Package metrics provides a Prometheus metrics registry for HTTP and domain
// authentication events in AUSF, backed by prometheus/client_golang.
package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// AuthRecorder records domain authentication events.
// Implemented by [Registry]; can be stubbed in tests.
type AuthRecorder interface {
	RecordAuthInitiated(authType string)
	RecordAuthConfirmed(authType string)
	RecordAuthFailed(cause string)
	RecordSyncFailure(authType string)
	RecordAuthDuration(authType string, durationSeconds float64)
	SetCertExpiryHours(hours float64)
}

// Registry collects HTTP and auth-domain metrics using prometheus/client_golang.
type Registry struct {
	reg             *prometheus.Registry
	httpRequests    *prometheus.CounterVec
	httpDuration    *prometheus.HistogramVec
	authInitiated   *prometheus.CounterVec
	authSuccess     *prometheus.CounterVec
	authRejected    *prometheus.CounterVec
	syncFailure     *prometheus.CounterVec
	authDuration    *prometheus.HistogramVec
	certExpiryHours prometheus.Gauge
}

// NewRegistry returns an initialised Registry backed by an isolated prometheus registry.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()

	httpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ausf_http_requests_total",
		Help: "Total HTTP requests handled by AUSF.",
	}, []string{"method", "route", "status"})

	httpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ausf_http_request_duration_seconds",
		Help:    "Request duration for AUSF handlers.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	authInitiated := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ausf_auth_initiated_total",
		Help: "Total authentication sessions initiated by type.",
	}, []string{"auth_type"})

	authSuccess := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ausf_auth_success_total",
		Help: "Total authentication sessions confirmed successfully by type.",
	}, []string{"auth_type"})

	authRejected := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ausf_auth_rejected_total",
		Help: "Total authentication rejections by cause.",
	}, []string{"cause"})

	syncFailure := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ausf_sync_failure_total",
		Help: "Total 5G AKA synchronisation failures by auth type.",
	}, []string{"auth_type"})

	authDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ausf_auth_duration_seconds",
		Help:    "Duration of authentication operations (initiate/confirm) in seconds.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
	}, []string{"auth_type"})

	certExpiryHours := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ausf_cert_expiry_hours",
		Help: "Hours until the TLS certificate expires. Alert when below 168 (7 days).",
	})

	reg.MustRegister(
		httpRequests,
		httpDuration,
		authInitiated,
		authSuccess,
		authRejected,
		syncFailure,
		authDuration,
		certExpiryHours,
	)

	return &Registry{
		reg:             reg,
		httpRequests:    httpRequests,
		httpDuration:    httpDuration,
		authInitiated:   authInitiated,
		authSuccess:     authSuccess,
		authRejected:    authRejected,
		syncFailure:     syncFailure,
		authDuration:    authDuration,
		certExpiryHours: certExpiryHours,
	}
}

// Gatherer returns the prometheus.Gatherer for use with promhttp.HandlerFor.
func (r *Registry) Gatherer() prometheus.Gatherer {
	return r.reg
}

// ObserveHTTP records a single HTTP request.
func (r *Registry) ObserveHTTP(method, route string, statusCode int, durationSeconds float64) {
	status := fmt.Sprintf("%d", statusCode)
	r.httpRequests.WithLabelValues(method, route, status).Inc()
	r.httpDuration.WithLabelValues(method, route).Observe(durationSeconds)
}

// RecordAuthInitiated increments the counter for initiated authentication sessions.
func (r *Registry) RecordAuthInitiated(authType string) {
	r.authInitiated.WithLabelValues(authType).Inc()
}

// RecordAuthConfirmed increments the counter for successfully confirmed authentication sessions.
func (r *Registry) RecordAuthConfirmed(authType string) {
	r.authSuccess.WithLabelValues(authType).Inc()
}

// RecordAuthFailed increments the counter for authentication rejections.
func (r *Registry) RecordAuthFailed(cause string) {
	r.authRejected.WithLabelValues(cause).Inc()
}

// RecordSyncFailure increments the counter for 5G AKA synchronisation failures.
func (r *Registry) RecordSyncFailure(authType string) {
	r.syncFailure.WithLabelValues(authType).Inc()
}

// RecordAuthDuration observes the duration of a single authentication operation.
func (r *Registry) RecordAuthDuration(authType string, durationSeconds float64) {
	r.authDuration.WithLabelValues(authType).Observe(durationSeconds)
}

// SetCertExpiryHours updates the TLS certificate expiry gauge.
func (r *Registry) SetCertExpiryHours(hours float64) {
	r.certExpiryHours.Set(hours)
}
