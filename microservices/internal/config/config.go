package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Host                           string
	Port                           string
	TLSCertFile                    string
	TLSKeyFile                     string
	SBIBearerToken                 string
	ControlPlaneBaseURL            string
	ControlPlaneBearerToken        string
	ControlPlaneCACertFile         string
	ControlPlaneInsecureSkipVerify bool
	NamfBaseURL                    string
	NamfBearerToken                string
	NamfCACertFile                 string
	NamfInsecureSkipVerify         bool
	AuthContextStoreFile           string
	AuthContextTTLSeconds          int
	BreakerFailures                int
	BreakerTimeoutSeconds          int
	OTLPEndpoint                   string
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
		SBIBearerToken:                 os.Getenv("AUSF_SBI_BEARER_TOKEN"),
		ControlPlaneBaseURL:            controlPlaneBaseURL,
		ControlPlaneBearerToken:        os.Getenv("CONTROL_PLANE_BEARER_TOKEN"),
		ControlPlaneCACertFile:         controlPlaneCACertFile,
		ControlPlaneInsecureSkipVerify: controlPlaneInsecureSkipVerify,
		NamfBaseURL:                    namfBaseURL,
		NamfBearerToken:                os.Getenv("AUSF_NAMF_BEARER_TOKEN"),
		NamfCACertFile:                 namfCACertFile,
		NamfInsecureSkipVerify:         namfInsecureSkipVerify,
		AuthContextStoreFile:           authContextStoreFile,
		AuthContextTTLSeconds:          authContextTTLSeconds,
		BreakerFailures:                breakerFailures,
		BreakerTimeoutSeconds:          breakerTimeoutSeconds,
		OTLPEndpoint:                   os.Getenv("AUSF_OTLP_ENDPOINT"),
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
