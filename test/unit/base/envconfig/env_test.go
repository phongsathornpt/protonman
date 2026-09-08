package envconfig_test

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/base/envconfig"
)

func TestConstants(t *testing.T) {
	tests := []struct {
		got  string
		want string
	}{
		{envconfig.Home, "PROTONMAN_HOME"},
		{envconfig.TrustProject, "PROTONMAN_TRUST_PROJECT"},
		{envconfig.SessionID, "PROTONMAN_SESSION_ID"},
		{envconfig.Sandbox, "PROTONMAN_SANDBOX"},
		{envconfig.Telemetry, "PROTONMAN_TELEMETRY"},
		{envconfig.DebugLog, "PROTONMAN_DEBUG_LOG"},
		{envconfig.ForceTTY, "PROTONMAN_FORCE_TTY"},
		{envconfig.LegacyTrustProject, "PROTON_TRUST_PROJECT"},
		{envconfig.LegacySessionID, "PROTON_SESSION_ID"},
		{envconfig.LegacySandbox, "PROTON_SANDBOX"},
		{envconfig.LegacyTelemetry, "PROTON_TELEMETRY"},
		{envconfig.LegacyDebugLog, "PROTON_DEBUG_LOG"},
		{envconfig.LegacyForceTTY, "PROTON_FORCE_TTY"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("constant = %q, want %q", tc.got, tc.want)
		}
	}
}

func TestHomeValueUsesCanonicalVariableOnly(t *testing.T) {
	t.Setenv(envconfig.Home, "  canonical  ")
	t.Setenv("PROTON_HOME", "legacy")
	if got := envconfig.Value(envconfig.Home); got != "canonical" {
		t.Fatalf("Value(Home) = %q, want canonical", got)
	}

	t.Setenv(envconfig.Home, "")
	if got := envconfig.Value(envconfig.Home); got != "" {
		t.Fatalf("Value(Home) = %q, want no legacy fallback", got)
	}
}

func TestValueForUnmappedVariableIsDirect(t *testing.T) {
	key := "TEST_PROTONMAN_VALUE_VAR"
	t.Setenv(key, "   custom_value   ")
	if got := envconfig.Value(key); got != "custom_value" {
		t.Fatalf("Value(%q) = %q, want custom_value", key, got)
	}

	t.Setenv(key, "")
	if got := envconfig.Value(key); got != "" {
		t.Fatalf("Value(%q) = %q, want empty", key, got)
	}
}

func TestBoolUsesLegacyFallback(t *testing.T) {
	t.Setenv(envconfig.TrustProject, "")
	t.Setenv(envconfig.LegacyTrustProject, "true")
	if !envconfig.Bool(envconfig.TrustProject) {
		t.Fatal("Bool(TrustProject) = false, want legacy true fallback")
	}

	t.Setenv(envconfig.TrustProject, "0")
	if envconfig.Bool(envconfig.TrustProject) {
		t.Fatal("Bool(TrustProject) ignored canonical false value")
	}
}

func TestTruthy(t *testing.T) {
	truthyCases := []string{"1", "true", "yes", "on", "TRUE", "Yes", "ON", " 1 ", " true "}
	for _, tc := range truthyCases {
		if !envconfig.Truthy(tc) {
			t.Errorf("Truthy(%q) = false, want true", tc)
		}
	}

	falsyCases := []string{"0", "false", "no", "off", "FALSE", "No", "off", "", "   ", "random", "2"}
	for _, tc := range falsyCases {
		if envconfig.Truthy(tc) {
			t.Errorf("Truthy(%q) = true, want false", tc)
		}
	}
}
