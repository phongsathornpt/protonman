package telemetry

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// ConfigureDebugLogger installs a process-wide logger for development diagnostics.
// When diagnostics are disabled, it installs a discard logger so internal slog
// records cannot leak onto an interactive terminal through slog.Default().
func ConfigureDebugLogger(destination string) (func(), error) {
	destination = strings.TrimSpace(destination)
	if isDebugLoggingDisabled(destination) {
		return installDebugLogger(io.Discard, nil), nil
	}

	writer, closer, err := debugWriter(destination)
	if err != nil {
		return nil, err
	}

	return installDebugLogger(writer, closer), nil
}

func installDebugLogger(writer io.Writer, closer io.Closer) func() {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	var restoreOnce sync.Once
	return func() {
		restoreOnce.Do(func() {
			slog.SetDefault(previous)
			if closer != nil {
				_ = closer.Close()
			}
		})
	}
}

func isDebugLoggingDisabled(destination string) bool {
	switch strings.ToLower(destination) {
	case "", "off", "false", "0":
		return true
	default:
		return false
	}
}

func debugWriter(destination string) (io.Writer, io.Closer, error) {
	if strings.EqualFold(destination, "stderr") {
		return os.Stderr, nil, nil
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return file, file, nil
}
