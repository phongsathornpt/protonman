package envconfig_test

import (
	"testing"

	"github.com/phongsathornpt/proton/internal/base/envconfig"
)

func TestConstants(t *testing.T) {
	tests := []struct {
		got  string
		want string
	}{
		{envconfig.Home, "PROTON_HOME"},
		{envconfig.TrustProject, "PROTON_TRUST_PROJECT"},
		{envconfig.SessionID, "PROTON_SESSION_ID"},
		{envconfig.Sandbox, "PROTON_SANDBOX"},
		{envconfig.Telemetry, "PROTON_TELEMETRY"},
		{envconfig.DebugLog, "PROTON_DEBUG_LOG"},
		{envconfig.ForceTTY, "PROTON_FORCE_TTY"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("constant = %q, want %q", tc.got, tc.want)
		}
	}
}

func TestValue(t *testing.T) {
	key := "TEST_PROTON_VALUE_VAR"
	t.Setenv(key, "   custom_value   ")
	if got := envconfig.Value(key); got != "custom_value" {
		t.Fatalf("Value(%q) = %q, want custom_value", key, got)
	}

	t.Setenv(key, "")
	if got := envconfig.Value(key); got != "" {
		t.Fatalf("Value(%q) = %q, want empty", key, got)
	}
}

func TestBool(t *testing.T) {
	key := "TEST_PROTON_BOOL_VAR"
	t.Setenv(key, "true")
	if !envconfig.Bool(key) {
		t.Fatalf("Bool(%q) = false, want true", key)
	}

	t.Setenv(key, "0")
	if envconfig.Bool(key) {
		t.Fatalf("Bool(%q) = true, want false", key)
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
