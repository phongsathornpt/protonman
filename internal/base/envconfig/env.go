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
	// Icons selects the TUI icon capability profile: auto, unicode, or ascii.
	Icons = "PROTONMAN_ICONS"
	// ReducedMotion replaces the TUI's self-running animation (the busy spinner
	// and the caret blink) with static indicators that carry the same meaning.
	// Functional signals, including activity text and retry countdowns, stay live.
	ReducedMotion = "PROTONMAN_REDUCED_MOTION"
	// InstallDir overrides the directory `protonman update` installs into.
	InstallDir = "PROTONMAN_INSTALL_DIR"
	// PinnedVersion pins the release tag `protonman update` installs.
	PinnedVersion = "PROTONMAN_VERSION"
)

// Value reads one canonical Protonman environment variable.
func Value(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

// DirectValue is retained as the explicit raw-name accessor for callers that
// intentionally supply an environment variable name at runtime.
func DirectValue(name string) string {
	return Value(name)
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
