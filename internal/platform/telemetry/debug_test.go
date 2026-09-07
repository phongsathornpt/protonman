package telemetry

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestConfigureDebugLoggerWritesJSONAndRestores(t *testing.T) {
	path := t.TempDir() + "/debug.jsonl"
	previous := slog.Default()

	restore, err := ConfigureDebugLogger(path)
	if err != nil {
		t.Fatalf("ConfigureDebugLogger() error = %v", err)
	}
	slog.Debug("debug test event", "phase", "test")
	restore()
	restore()

	if slog.Default() != previous {
		t.Fatal("ConfigureDebugLogger() did not restore the previous logger")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	logLine := string(data)
	for _, expected := range []string{
		`"msg":"debug test event"`,
		`"phase":"test"`,
	} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("debug log does not contain %q: %s", expected, logLine)
		}
	}
}

func TestConfigureDebugLoggerDisabled(t *testing.T) {
	previous := slog.Default()

	for _, destination := range []string{"", "off", "false", "0"} {
		restore, err := ConfigureDebugLogger(destination)
		if err != nil {
			t.Fatalf("ConfigureDebugLogger(%q) error = %v", destination, err)
		}
		restore()
		if slog.Default() != previous {
			t.Fatalf("ConfigureDebugLogger(%q) changed the logger", destination)
		}
	}
}

func TestFingerprintIsStableAndShort(t *testing.T) {
	first := Fingerprint("private command")
	second := Fingerprint("private command")
	other := Fingerprint("another command")

	if first != second {
		t.Fatalf("Fingerprint() is not stable: %q != %q", first, second)
	}
	if first == other {
		t.Fatalf("Fingerprint() collision for test values: %q", first)
	}
	if len(first) != 16 {
		t.Fatalf("Fingerprint() length = %d, want 16", len(first))
	}
}
