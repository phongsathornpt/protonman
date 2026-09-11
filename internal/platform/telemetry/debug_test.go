package telemetry

import (
	"bytes"
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

func TestConfigureDebugLoggerDisabledSuppressesDefaultStderrLogging(t *testing.T) {
	original := slog.Default()
	defer slog.SetDefault(original)

	for _, destination := range []string{"", "off", "false", "0"} {
		var output bytes.Buffer
		previous := slog.New(slog.NewTextHandler(&output, nil))
		slog.SetDefault(previous)

		restore, err := ConfigureDebugLogger(destination)
		if err != nil {
			t.Fatalf("ConfigureDebugLogger(%q) error = %v", destination, err)
		}
		if slog.Default() == previous {
			t.Fatalf("ConfigureDebugLogger(%q) left the default stderr-capable logger installed", destination)
		}
		slog.Warn("must stay silent")
		if output.Len() != 0 {
			t.Fatalf("ConfigureDebugLogger(%q) leaked disabled log output: %q", destination, output.String())
		}

		restore()
		if slog.Default() != previous {
			t.Fatalf("ConfigureDebugLogger(%q) did not restore the previous logger", destination)
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
