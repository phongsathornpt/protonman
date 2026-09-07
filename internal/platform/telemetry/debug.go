package telemetry

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// ConfigureDebugLogger installs a process-wide debug logger for development diagnostics.
// An empty, off, false, or 0 destination leaves the current logger unchanged.
func ConfigureDebugLogger(destination string) (func(), error) {
	destination = strings.TrimSpace(destination)
	if isDebugLoggingDisabled(destination) {
		return func() {}, nil
	}

	writer, closer, err := debugWriter(destination)
	if err != nil {
		return nil, err
	}

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
	}, nil
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
