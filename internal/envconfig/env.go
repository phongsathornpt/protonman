package envconfig

import (
	"os"
	"strings"
)

const (
	Home         = "PROTON_HOME"
	TrustProject = "PROTON_TRUST_PROJECT"
	SessionID    = "PROTON_SESSION_ID"
	Sandbox      = "PROTON_SANDBOX"
	Telemetry    = "PROTON_TELEMETRY"
	DebugLog     = "PROTON_DEBUG_LOG"
	ForceTTY     = "PROTON_FORCE_TTY"
)

func Value(name string) string {
	return strings.TrimSpace(os.Getenv(name))
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
