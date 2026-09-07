package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
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

func TestSlogObserverProtectionEventsAreRedactedAndCounted(t *testing.T) {
	var output bytes.Buffer
	observer, err := NewSlogObserver(slog.New(slog.NewJSONHandler(&output, nil)))
	if err != nil {
		t.Fatal(err)
	}
	observer.ObserveProtection(context.Background(), toolcall.ProtectionEvent{
		Kind: toolcall.ProtectionCallSuppressed, ToolName: "bash", ToolKind: permission.ToolBash,
		Reason: "permission_retry", Fingerprint: "0123456789abcdef", RepeatCount: 1,
		ErrorCode: tool.ErrorCodePermissionDenied,
	})
	observer.ObserveProtection(context.Background(), toolcall.ProtectionEvent{Kind: toolcall.ProtectionPermissionSuppressed})
	observer.Observe(context.Background(), toolcall.Event{Kind: toolcall.EventCallFailed, ErrorCode: tool.ErrorCodeStaleContinuation})
	counters := observer.Counters()
	if counters["tool_call_suppressed_total"] != 1 {
		t.Fatalf("suppressed counter = %d", counters["tool_call_suppressed_total"])
	}
	if counters["tool_permission_retry_suppressed_total"] != 1 {
		t.Fatalf("permission counter = %d", counters["tool_permission_retry_suppressed_total"])
	}
	if counters["tool_continuation_stale_total"] != 1 {
		t.Fatalf("stale counter = %d", counters["tool_continuation_stale_total"])
	}
	logLine := output.String()
	for _, expected := range []string{`"event_kind":"tool_call_suppressed"`, `"semantic_fingerprint":"0123456789abcdef"`, `"reason":"permission_retry"`} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("log missing %q: %s", expected, logLine)
		}
	}
	for _, forbidden := range []string{"rm -rf", "/private/path", "secret-output", `{"command"`} {
		if strings.Contains(logLine, forbidden) {
			t.Fatalf("log contains sensitive value %q: %s", forbidden, logLine)
		}
	}
}
