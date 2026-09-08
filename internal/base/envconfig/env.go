package envconfig

import (
	"os"
	"strings"
)

const (
	Home         = "PROTONMAN_HOME"
	TrustProject = "PROTONMAN_TRUST_PROJECT"
	SessionID    = "PROTONMAN_SESSION_ID"
	Sandbox      = "PROTONMAN_SANDBOX"
	Telemetry    = "PROTONMAN_TELEMETRY"
	DebugLog     = "PROTONMAN_DEBUG_LOG"
	ForceTTY     = "PROTONMAN_FORCE_TTY"

	LegacyHome         = "PROTON_HOME"
	LegacyTrustProject = "PROTON_TRUST_PROJECT"
	LegacySessionID    = "PROTON_SESSION_ID"
	LegacySandbox      = "PROTON_SANDBOX"
	LegacyTelemetry    = "PROTON_TELEMETRY"
	LegacyDebugLog     = "PROTON_DEBUG_LOG"
	LegacyForceTTY     = "PROTON_FORCE_TTY"
)

var legacyNames = map[string]string{
	Home:         LegacyHome,
	TrustProject: LegacyTrustProject,
	SessionID:    LegacySessionID,
	Sandbox:      LegacySandbox,
	Telemetry:    LegacyTelemetry,
	DebugLog:     LegacyDebugLog,
	ForceTTY:     LegacyForceTTY,
}

// DirectValue reads one environment variable without compatibility fallback.
func DirectValue(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

// Value returns the canonical Protonman environment value, falling back to the
// corresponding legacy PROTON_* variable when the canonical value is unset.
func Value(name string) string {
	if value := DirectValue(name); value != "" {
		return value
	}
	if legacy := legacyNames[name]; legacy != "" {
		return DirectValue(legacy)
	}
	return ""
}

func Bool(name string) bool {
	return Truthy(Value(name))
}

func Truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
