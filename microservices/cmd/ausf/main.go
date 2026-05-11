package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexey/ausf/microservices/internal/api"
	"github.com/alexey/ausf/microservices/internal/config"
	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/metrics"
	"github.com/alexey/ausf/microservices/internal/namf"
	"github.com/alexey/ausf/microservices/internal/oauth2"
	"github.com/alexey/ausf/microservices/internal/service"
	"github.com/alexey/ausf/microservices/internal/tlsutil"
	"github.com/alexey/ausf/microservices/internal/tracing"
	"github.com/alexey/ausf/microservices/internal/transport"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	log.SetFlags(0) // let the JSON lines carry their own timestamp

	appConfig := config.Load()

	reg := metrics.NewRegistry()
	api.SetDefaultRegistry(reg)

	exporter := tracing.NewExporter(appConfig.OTLPEndpoint, "ausf-microservice")
	defer exporter.Shutdown()
	tracer := tracing.NewTracer("ausf-microservice", exporter)
	api.SetTracer(tracer)

	certReload := time.Duration(appConfig.TLSCertReloadIntervalSeconds) * time.Second

	controlPlaneClient, err := controlplane.NewClientWithTLSAndBreaker(
		appConfig.ControlPlaneBaseURL,
		transport.TLSClientConfig{
			CACertFile:         appConfig.ControlPlaneCACertFile,
			InsecureSkipVerify: appConfig.ControlPlaneInsecureSkipVerify,
			ForceH2C:           appConfig.ControlPlaneH2CEnabled,
			ClientCertFile:     appConfig.ControlPlaneClientCertFile,
			ClientKeyFile:      appConfig.ControlPlaneClientKeyFile,
			CertReloadInterval: certReload,
			OCSPEnabled:        appConfig.OCSPEnabled,
		},
		appConfig.BreakerFailures,
		time.Duration(appConfig.BreakerTimeoutSeconds)*time.Second,
		appConfig.ControlPlaneBearerToken,
	)
	if err != nil {
		logMain("FATAL", "failed to initialize control-plane TLS client", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	namfClient, err := namf.NewClientWithTLS(appConfig.NamfBaseURL, transport.TLSClientConfig{
		CACertFile:         appConfig.NamfCACertFile,
		InsecureSkipVerify: appConfig.NamfInsecureSkipVerify,
		ForceH2C:           appConfig.NamfH2CEnabled,
		ClientCertFile:     appConfig.NamfClientCertFile,
		ClientKeyFile:      appConfig.NamfClientKeyFile,
		CertReloadInterval: certReload,
		OCSPEnabled:        appConfig.OCSPEnabled,
	}, appConfig.NamfBearerToken)
	if err != nil {
		logMain("FATAL", "failed to initialize Namf TLS client", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	store, err := service.NewFileAuthContextStore(appConfig.AuthContextStoreFile)
	if err != nil {
		logMain("FATAL", "failed to initialize auth context store", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	authService := service.NewAuthServiceWithStoreAndTTL(
		controlPlaneClient,
		namfClient,
		store,
		time.Duration(appConfig.AuthContextTTLSeconds)*time.Second,
	)
	authService.SetMetricsRecorder(reg)

	var jwksProvider *oauth2.JWKSProvider
	if appConfig.NRFJWKSUrl != "" {
		ttl := time.Duration(appConfig.JWKSCacheTTLSeconds) * time.Second
		jwksProvider = oauth2.NewJWKSProvider(appConfig.NRFJWKSUrl, ttl, nil)
	}

	var introspector *oauth2.Introspector
	if appConfig.NRFIntrospectionURL != "" {
		cacheTTL := time.Duration(appConfig.IntrospectionCacheTTLSeconds) * time.Second
		introspector = oauth2.NewIntrospector(
			appConfig.NRFIntrospectionURL,
			appConfig.OAuth2ClientID,
			appConfig.OAuth2ClientSecret,
			cacheTTL,
			nil,
		)
	}

	// Wire outbound CCF token sources when the NRF token endpoint is configured.
	if appConfig.NRFTokenURL != "" && appConfig.OAuth2ClientID != "" {
		cpTokenSource := oauth2.NewCCFTokenProvider(
			appConfig.NRFTokenURL,
			appConfig.OAuth2ClientID,
			appConfig.OAuth2ClientSecret,
			appConfig.ControlPlaneCCFScope,
			nil,
		)
		controlPlaneClient.WithTokenSource(cpTokenSource)

		namfTokenSource := oauth2.NewCCFTokenProvider(
			appConfig.NRFTokenURL,
			appConfig.OAuth2ClientID,
			appConfig.OAuth2ClientSecret,
			appConfig.NamfCCFScope,
			nil,
		)
		namfClient.WithTokenSource(namfTokenSource)
	}

	handler := api.NewHandlerWithAuthorization(authService, api.AuthorizationConfig{
		BearerToken: appConfig.SBIBearerToken,
		OAuth2: api.OAuth2Config{
			Enabled:               appConfig.OAuth2Enabled,
			ExpectedAudience:      appConfig.OAuth2ExpectedAudience,
			ExpectedScope:         appConfig.OAuth2ExpectedScope,
			JWKSProvider:          jwksProvider,
			IntrospectionProvider: introspector,
		},
	})

	server := &http.Server{
		Addr:    appConfig.Address(),
		Handler: handler.Routes(),
	}

	logMain("INFO", "AUSF microservice starting", map[string]any{
		"address":       appConfig.Address(),
		"control_plane": appConfig.ControlPlaneBaseURL,
		"namf":          appConfig.NamfBaseURL,
		"tls_enabled":   appConfig.TLSEnabled(),
	})

	// Start serving in a goroutine; signal handler below handles shutdown.
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- serve(server, appConfig)
	}()

	// Block until SIGTERM or SIGINT, then drain existing connections.
	// For HTTP/2, server.Shutdown sends a GOAWAY frame (RFC 9113 §6.8) before
	// closing, allowing in-flight requests to complete on the client side.
	stopCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			logMain("FATAL", "server failed", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
	case <-stopCtx.Done():
		stop() // release signal resources promptly
		logMain("INFO", "shutdown signal received, draining connections", map[string]any{
			"timeout_seconds": appConfig.ShutdownTimeoutSeconds,
		})
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			time.Duration(appConfig.ShutdownTimeoutSeconds)*time.Second,
		)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logMain("ERROR", "graceful shutdown timed out", map[string]any{"error": err.Error()})
		}
		<-serveErr // wait for ListenAndServe* to return
		logMain("INFO", "server stopped", map[string]any{})
	}
}

func serve(server *http.Server, appConfig config.Config) error {
	if appConfig.TLSEnabled() {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

		// Hot-reload: GetCertificate re-reads cert/key from disk when the TTL
		// expires so that cert rotation takes effect without a process restart.
		certReload := time.Duration(appConfig.TLSCertReloadIntervalSeconds) * time.Second
		certLoader := tlsutil.NewCertLoader(appConfig.TLSCertFile, appConfig.TLSKeyFile, certReload)
		// Eagerly load once to fail fast on misconfiguration.
		if _, err := certLoader.Get(); err != nil {
			return fmt.Errorf("server TLS: %w", err)
		}
		tlsCfg.GetCertificate = certLoader.GetCertificate

		if appConfig.MTLSEnabled {
			tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
			if appConfig.TLSClientCACertFile != "" {
				pool, err := loadCACertPool(appConfig.TLSClientCACertFile)
				if err != nil {
					return fmt.Errorf("mTLS inbound CA: %w", err)
				}
				tlsCfg.ClientCAs = pool
			}
			if appConfig.OCSPEnabled {
				checker := tlsutil.NewOCSPChecker()
				tlsCfg.VerifyPeerCertificate = checker.VerifyPeerCertificate
			}
		}

		server.TLSConfig = tlsCfg
		// Go's net/http automatically negotiates HTTP/2 via ALPN when serving TLS.
		// Empty cert/key args: GetCertificate already handles certificate loading.
		return server.ListenAndServeTLS("", "")
	}
	// Non-TLS: wrap with h2c to support HTTP/2 cleartext (required for 5G SBI per TS 29.500).
	server.Handler = h2c.NewHandler(server.Handler, &http2.Server{})
	return server.ListenAndServe()
}

// loadCACertPool reads a PEM CA file and returns a cert pool.
func loadCACertPool(caFile string) (*x509.CertPool, error) {
	caBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert file %q: %w", caFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("no valid PEM certificates found in %q", caFile)
	}
	return pool, nil
}

func logMain(level, msg string, fields map[string]any) {
	fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["level"] = level
	fields["msg"] = msg
	data, _ := json.Marshal(fields)
	_, _ = fmt.Fprintln(os.Stderr, string(data))
}
