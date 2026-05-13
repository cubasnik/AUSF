package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Host        string
	Port        string
	TLSCertFile string
	TLSKeyFile  string
	// MTLSEnabled enables inbound client-certificate verification (mTLS).
	MTLSEnabled bool
	// TLSClientCACertFile is the CA used to verify inbound client certificates.
	TLSClientCACertFile    string
	SBIBearerToken         string
	OAuth2Enabled          bool
	OAuth2ExpectedAudience string
	OAuth2ExpectedScope    string
	NRFJWKSUrl             string // NRF JWKS endpoint for JWT signature verification
	JWKSCacheTTLSeconds    int    // JWKS cache TTL; default 300 s
	// NRFIntrospectionURL is the RFC 7662 token introspection endpoint on the NRF.
	// When set, every inbound JWT is checked for revocation. Best-effort: an
	// unreachable endpoint does not block authentication.
	NRFIntrospectionURL          string
	IntrospectionCacheTTLSeconds int // introspection cache TTL; default 60 s
	// NRFTokenURL is the NRF OAuth2 token endpoint for the CCF (client_credentials grant).
	NRFTokenURL                    string
	OAuth2ClientID                 string
	OAuth2ClientSecret             string
	ControlPlaneBaseURL            string
	ControlPlaneBearerToken        string
	ControlPlaneCACertFile         string
	ControlPlaneInsecureSkipVerify bool
	ControlPlaneH2CEnabled         bool // force cleartext HTTP/2 toward the Java control-plane
	// ControlPlaneCCFScope is the OAuth2 scope requested when obtaining a token for the control-plane.
	ControlPlaneCCFScope string
	// ControlPlaneClientCertFile / ControlPlaneClientKeyFile enable outbound mTLS toward the control-plane.
	ControlPlaneClientCertFile string
	ControlPlaneClientKeyFile  string
	NamfBaseURL                string
	NamfBearerToken            string
	NamfCACertFile             string
	NamfInsecureSkipVerify     bool
	NamfH2CEnabled             bool // force cleartext HTTP/2 toward NamF
	// NamfCCFScope is the OAuth2 scope requested when obtaining a token for Namf.
	NamfCCFScope string
	// NamfClientCertFile / NamfClientKeyFile enable outbound mTLS toward Namf.
	NamfClientCertFile    string
	NamfClientKeyFile     string
	AuthContextStoreFile  string
	AuthContextTTLSeconds int
	BreakerFailures       int
	BreakerTimeoutSeconds int
	OTLPEndpoint          string
	// OCSPEnabled enables OCSP revocation checking for outbound TLS peer
	// certificates and for inbound peer certificates when mTLS is active.
	OCSPEnabled bool
	// TLSCertReloadIntervalSeconds controls how often the server and outbound
	// client certificates are re-read from disk.  0 means use the default (30 s).
	TLSCertReloadIntervalSeconds int
	// ShutdownTimeoutSeconds is the maximum time allowed for graceful shutdown
	// (draining in-flight requests + sending HTTP/2 GOAWAY per RFC 9113 §6.8).
	// 0 means use the default (30 s).
	ShutdownTimeoutSeconds int
	// NamfQueueMaxAttempts is the maximum number of asynchronous retry attempts
	// for Namf notifications that fail all synchronous delivery retries.
	// 0 means use the default (10).
	NamfQueueMaxAttempts int
	// NamfQueueBackend selects the async retry queue backend.
	// Accepted values: "memory" (default), "redis".
	NamfQueueBackend string
	// NamfRedisURL is the Redis connection URL used when NamfQueueBackend="redis".
	// Example: redis://localhost:6379/0
	NamfRedisURL string
	// OverloadThreshold is the maximum number of concurrent in-flight requests
	// before the NF Overload Control middleware starts shedding load with HTTP 503.
	// 0 means use the built-in default of 500.
	OverloadThreshold int
}

func Load() Config {
	host := os.Getenv("AUSF_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	port := os.Getenv("AUSF_PORT")
	if port == "" {
		port = "8080"
	}

	controlPlaneBaseURL := os.Getenv("CONTROL_PLANE_BASE_URL")
	if controlPlaneBaseURL == "" {
		controlPlaneBaseURL = "http://127.0.0.1:8081"
	}

	namfBaseURL := os.Getenv("AUSF_NAMF_BASE_URL")
	tlsCertFile := os.Getenv("AUSF_TLS_CERT_FILE")
	tlsKeyFile := os.Getenv("AUSF_TLS_KEY_FILE")
	controlPlaneCACertFile := os.Getenv("CONTROL_PLANE_TLS_CA_CERT_FILE")
	controlPlaneInsecureSkipVerify := parseBoolEnv("CONTROL_PLANE_TLS_INSECURE_SKIP_VERIFY")
	namfCACertFile := os.Getenv("AUSF_NAMF_TLS_CA_CERT_FILE")
	namfInsecureSkipVerify := parseBoolEnv("AUSF_NAMF_TLS_INSECURE_SKIP_VERIFY")
	authContextStoreFile := os.Getenv("AUSF_AUTH_CONTEXT_STORE_FILE")
	if authContextStoreFile == "" {
		authContextStoreFile = "/tmp/ausf-auth-contexts.json"
	}

	authContextTTLSeconds := 900
	if rawTTL := os.Getenv("AUSF_AUTH_CONTEXT_TTL_SECONDS"); rawTTL != "" {
		if parsedTTL, err := strconv.Atoi(rawTTL); err == nil && parsedTTL > 0 {
			authContextTTLSeconds = parsedTTL
		}
	}

	breakerFailures := 5
	if rawFailures := os.Getenv("AUSF_CONTROL_PLANE_BREAKER_FAILURES"); rawFailures != "" {
		if parsedFailures, err := strconv.Atoi(rawFailures); err == nil && parsedFailures > 0 {
			breakerFailures = parsedFailures
		}
	}

	breakerTimeoutSeconds := 10
	if rawTimeout := os.Getenv("AUSF_CONTROL_PLANE_BREAKER_TIMEOUT_SECONDS"); rawTimeout != "" {
		if parsedTimeout, err := strconv.Atoi(rawTimeout); err == nil && parsedTimeout > 0 {
			breakerTimeoutSeconds = parsedTimeout
		}
	}

	return Config{
		Host:                           host,
		Port:                           port,
		TLSCertFile:                    tlsCertFile,
		TLSKeyFile:                     tlsKeyFile,
		MTLSEnabled:                    parseBoolEnv("AUSF_MTLS_ENABLED"),
		TLSClientCACertFile:            os.Getenv("AUSF_TLS_CLIENT_CA_CERT_FILE"),
		SBIBearerToken:                 os.Getenv("AUSF_SBI_BEARER_TOKEN"),
		OAuth2Enabled:                  parseBoolEnv("AUSF_OAUTH2_ENABLED"),
		OAuth2ExpectedAudience:         os.Getenv("AUSF_OAUTH2_AUDIENCE"),
		OAuth2ExpectedScope:            os.Getenv("AUSF_OAUTH2_SCOPE"),
		NRFJWKSUrl:                     os.Getenv("AUSF_NRF_JWKS_URL"),
		JWKSCacheTTLSeconds:            parseIntEnvDefault("AUSF_JWKS_CACHE_TTL_SECONDS", 300),
		NRFIntrospectionURL:            os.Getenv("AUSF_NRF_INTROSPECTION_URL"),
		IntrospectionCacheTTLSeconds:   parseIntEnvDefault("AUSF_INTROSPECTION_CACHE_TTL_SECONDS", 60),
		NRFTokenURL:                    os.Getenv("AUSF_NRF_TOKEN_URL"),
		OAuth2ClientID:                 os.Getenv("AUSF_OAUTH2_CLIENT_ID"),
		OAuth2ClientSecret:             os.Getenv("AUSF_OAUTH2_CLIENT_SECRET"),
		ControlPlaneBaseURL:            controlPlaneBaseURL,
		ControlPlaneBearerToken:        os.Getenv("CONTROL_PLANE_BEARER_TOKEN"),
		ControlPlaneCACertFile:         controlPlaneCACertFile,
		ControlPlaneInsecureSkipVerify: controlPlaneInsecureSkipVerify,
		ControlPlaneH2CEnabled:         parseBoolEnv("CONTROL_PLANE_H2C_ENABLED"),
		ControlPlaneCCFScope:           os.Getenv("CONTROL_PLANE_OAUTH2_SCOPE"),
		ControlPlaneClientCertFile:     os.Getenv("CONTROL_PLANE_TLS_CLIENT_CERT_FILE"),
		ControlPlaneClientKeyFile:      os.Getenv("CONTROL_PLANE_TLS_CLIENT_KEY_FILE"),
		NamfBaseURL:                    namfBaseURL,
		NamfBearerToken:                os.Getenv("AUSF_NAMF_BEARER_TOKEN"),
		NamfCACertFile:                 namfCACertFile,
		NamfInsecureSkipVerify:         namfInsecureSkipVerify,
		NamfH2CEnabled:                 parseBoolEnv("AUSF_NAMF_H2C_ENABLED"),
		NamfCCFScope:                   os.Getenv("AUSF_NAMF_OAUTH2_SCOPE"),
		NamfClientCertFile:             os.Getenv("AUSF_NAMF_TLS_CLIENT_CERT_FILE"),
		NamfClientKeyFile:              os.Getenv("AUSF_NAMF_TLS_CLIENT_KEY_FILE"),
		AuthContextStoreFile:           authContextStoreFile,
		AuthContextTTLSeconds:          authContextTTLSeconds,
		BreakerFailures:                breakerFailures,
		BreakerTimeoutSeconds:          breakerTimeoutSeconds,
		OTLPEndpoint:                   os.Getenv("AUSF_OTLP_ENDPOINT"),
		OCSPEnabled:                    parseBoolEnv("AUSF_OCSP_ENABLED"),
		TLSCertReloadIntervalSeconds:   parseIntEnvDefault("AUSF_TLS_CERT_RELOAD_INTERVAL_SECONDS", 30),
		ShutdownTimeoutSeconds:         parseIntEnvDefault("AUSF_SHUTDOWN_TIMEOUT_SECONDS", 30),
		NamfQueueMaxAttempts:           parseIntEnvDefault("AUSF_NAMF_QUEUE_MAX_ATTEMPTS", 10),
		NamfQueueBackend:               os.Getenv("AUSF_NAMF_QUEUE_BACKEND"),
		NamfRedisURL:                   os.Getenv("AUSF_NAMF_REDIS_URL"),
		OverloadThreshold:              parseIntEnvDefault("AUSF_OVERLOAD_THRESHOLD", 500),
	}
}

func (config Config) Address() string {
	return fmt.Sprintf("%s:%s", config.Host, config.Port)
}

func (config Config) TLSEnabled() bool {
	return config.TLSCertFile != "" && config.TLSKeyFile != ""
}

func parseBoolEnv(name string) bool {
	value := os.Getenv(name)
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func parseIntEnvDefault(name string, defaultVal int) int {
	if raw := os.Getenv(name); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	return defaultVal
}
