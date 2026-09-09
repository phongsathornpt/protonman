package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
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

func TestSlogObserverCountsRecoveryLifecycle(t *testing.T) {
	var output bytes.Buffer
	observer, err := NewSlogObserver(slog.New(slog.NewJSONHandler(&output, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []toolcall.EventKind{toolcall.EventRecoveryAttempted, toolcall.EventRecoverySucceeded, toolcall.EventRecoveryFailed} {
		observer.Observe(context.Background(), toolcall.Event{Kind: kind, ToolName: "read", RecoveryAction: tool.RecoveryRestartPagination})
	}
	counters := observer.Counters()
	if counters["tool_recovery_attempt_total"] != 1 || counters["tool_recovery_success_total"] != 1 || counters["tool_recovery_failure_total"] != 1 {
		t.Fatalf("recovery counters = %#v", counters)
	}
	if logLine := output.String(); !strings.Contains(logLine, `"recovery_action":"restart_pagination"`) {
		t.Fatalf("recovery action missing from log: %s", logLine)
	}
}

func TestSlogObserverCountsRedactedAgentLifecycle(t *testing.T) {
	var output bytes.Buffer
	observer, err := NewSlogObserver(slog.New(slog.NewJSONHandler(&output, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"agent_wait_timeout", "agent_completed", "agent_failed", "agent_interrupted", "agent_resumed", "agent_persistence_failure"} {
		observer.ObserveAgent(context.Background(), kind, "strength-7", "turn-4", "strength")
	}
	counters := observer.Counters()
	for _, metric := range []string{"agent_wait_timeout_total", "agent_completed_total", "agent_failed_total", "agent_interrupted_total", "agent_resumed_total", "agent_persistence_failure_total"} {
		if counters[metric] != 1 {
			t.Fatalf("%s = %d, want 1; counters=%#v", metric, counters[metric], counters)
		}
	}
	logLine := output.String()
	for _, expected := range []string{`"event_kind":"agent_resumed"`, `"agent_id":"strength-7"`, `"parent_id":"turn-4"`, `"profile":"strength"`} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("agent log missing %q: %s", expected, logLine)
		}
	}
	for _, forbidden := range []string{"task text", "tool arguments", "model output"} {
		if strings.Contains(logLine, forbidden) {
			t.Fatalf("agent log contains forbidden value %q: %s", forbidden, logLine)
		}
	}
}
