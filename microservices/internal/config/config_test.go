package config

import "testing"

func TestLoadShouldUseBreakerDefaults(t *testing.T) {
	t.Setenv("AUSF_CONTROL_PLANE_BREAKER_FAILURES", "")
	t.Setenv("AUSF_CONTROL_PLANE_BREAKER_TIMEOUT_SECONDS", "")

	cfg := Load()

	if cfg.BreakerFailures != 5 {
		t.Fatalf("BreakerFailures = %d, want 5", cfg.BreakerFailures)
	}
	if cfg.BreakerTimeoutSeconds != 10 {
		t.Fatalf("BreakerTimeoutSeconds = %d, want 10", cfg.BreakerTimeoutSeconds)
	}
}

func TestLoadShouldParseBreakerEnvValues(t *testing.T) {
	t.Setenv("AUSF_CONTROL_PLANE_BREAKER_FAILURES", "7")
	t.Setenv("AUSF_CONTROL_PLANE_BREAKER_TIMEOUT_SECONDS", "25")

	cfg := Load()

	if cfg.BreakerFailures != 7 {
		t.Fatalf("BreakerFailures = %d, want 7", cfg.BreakerFailures)
	}
	if cfg.BreakerTimeoutSeconds != 25 {
		t.Fatalf("BreakerTimeoutSeconds = %d, want 25", cfg.BreakerTimeoutSeconds)
	}
}

func TestLoadShouldFallbackForInvalidBreakerEnvValues(t *testing.T) {
	t.Setenv("AUSF_CONTROL_PLANE_BREAKER_FAILURES", "0")
	t.Setenv("AUSF_CONTROL_PLANE_BREAKER_TIMEOUT_SECONDS", "-1")

	cfg := Load()

	if cfg.BreakerFailures != 5 {
		t.Fatalf("BreakerFailures = %d, want 5", cfg.BreakerFailures)
	}
	if cfg.BreakerTimeoutSeconds != 10 {
		t.Fatalf("BreakerTimeoutSeconds = %d, want 10", cfg.BreakerTimeoutSeconds)
	}
}
