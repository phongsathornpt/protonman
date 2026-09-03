package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

func TestSlogObserverWritesStructuredRedactedEvent(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	observer, err := NewSlogObserver(logger)
	if err != nil {
		t.Fatalf("NewSlogObserver() error = %v", err)
	}

	observer.Observe(context.Background(), toolcall.Event{
		Kind:          toolcall.EventCallFailed,
		CallID:        "call-1",
		ToolName:      "bash",
		ToolKind:      permission.ToolBash,
		Mode:          permission.ModeAsk,
		Decision:      permission.ActionDeny,
		ArgumentBytes: 42,
		ErrorCode:     tool.ErrorCodePermissionDenied,
	})

	logLine := output.String()
	for _, expected := range []string{
		`"event_kind":"tool_call_failed"`,
		`"tool_name":"bash"`,
		`"argument_bytes":42`,
		`"error_code":"permission_denied"`,
	} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("log does not contain %q: %s", expected, logLine)
		}
	}
	for _, forbidden := range []string{"command", "private/path", "secret-value"} {
		if strings.Contains(logLine, forbidden) {
			t.Fatalf("log contains forbidden value %q: %s", forbidden, logLine)
		}
	}
}

func TestNewSlogObserverRequiresLogger(t *testing.T) {
	_, err := NewSlogObserver(nil)
	if err == nil {
		t.Fatal("NewSlogObserver() error = nil, want required logger error")
	}
}
