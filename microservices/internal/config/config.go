package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Host                  string
	Port                  string
	ControlPlaneBaseURL   string
	NamfBaseURL           string
	AuthContextStoreFile  string
	AuthContextTTLSeconds int
	BreakerFailures       int
	BreakerTimeoutSeconds int
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

	return Config{Host: host, Port: port, ControlPlaneBaseURL: controlPlaneBaseURL, NamfBaseURL: namfBaseURL, AuthContextStoreFile: authContextStoreFile, AuthContextTTLSeconds: authContextTTLSeconds, BreakerFailures: breakerFailures, BreakerTimeoutSeconds: breakerTimeoutSeconds}
}

func (config Config) Address() string {
	return fmt.Sprintf("%s:%s", config.Host, config.Port)
}
