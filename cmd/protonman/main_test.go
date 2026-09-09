package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestMainRunHelp(t *testing.T) {
	ctx := context.Background()
	if err := run(ctx, []string{"--help"}); err != nil {
		t.Fatalf("run(--help) failed: %v", err)
	}
	if err := run(ctx, []string{"-h"}); err != nil {
		t.Fatalf("run(-h) failed: %v", err)
	}
}

func TestMainRunVersion(t *testing.T) {
	if err := run(context.Background(), []string{"--version"}); err != nil {
		t.Fatalf("run(--version) failed: %v", err)
	}
}

func TestMainRunInvalidFlags(t *testing.T) {
	ctx := context.Background()

	// Unknown flag
	err := run(ctx, []string{"--bogus-unknown-flag"})
	if err == nil {
		t.Fatal("expected error on unknown flag, got nil")
	}

	// Conflict: resume and new-session
	err = run(ctx, []string{"--resume", "--new-session"})
	if err == nil || !strings.Contains(err.Error(), "cannot specify both") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestMainRunInvalidConfigurations(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	work := t.TempDir()

	origHome := os.Getenv("PROTONMAN_HOME")
	defer func() {
		_ = os.Setenv("PROTONMAN_HOME", origHome)
	}()
	_ = os.Setenv("PROTONMAN_HOME", home)

	origWd, _ := os.Getwd()
	_ = os.Chdir(work)
	defer func() { _ = os.Chdir(origWd) }()

	// Invalid permission mode
	err := run(ctx, []string{"--permission-mode", "invalid_mode", "-p", "test"})
	if err == nil || (!strings.Contains(err.Error(), "unknown permission mode") && !strings.Contains(err.Error(), "invalid permission mode")) {
		t.Fatalf("expected invalid permission mode error, got: %v", err)
	}

	// Invalid agent profile
	err = run(ctx, []string{"--agent", "invalid_profile", "-p", "test"})
	if err == nil || !strings.Contains(err.Error(), "unknown agent profile") {
		t.Fatalf("expected invalid agent profile error, got: %v", err)
	}

	// Invalid sandbox profile
	err = run(ctx, []string{"--sandbox", "invalid_sandbox", "-p", "test"})
	if err == nil || !strings.Contains(err.Error(), "invalid sandbox") {
		t.Fatalf("expected invalid sandbox error, got: %v", err)
	}

	// Invalid telemetry
	origTelem := os.Getenv("PROTON_TELEMETRY")
	defer func() { _ = os.Setenv("PROTON_TELEMETRY", origTelem) }()
	_ = os.Setenv("PROTON_TELEMETRY", "invalid_telemetry_sink")
	err = run(ctx, []string{"-y", "-p", `/call bash {"command":"echo hi"}`})
	if err == nil || !strings.Contains(err.Error(), "unsupported PROTONMAN_TELEMETRY") {
		t.Fatalf("expected unsupported telemetry error, got: %v", err)
	}
}

func TestMainRunHeadlessRefusalAndExecution(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	work := t.TempDir()

	origHome := os.Getenv("PROTONMAN_HOME")
	defer func() {
		_ = os.Setenv("PROTONMAN_HOME", origHome)
	}()
	_ = os.Setenv("PROTONMAN_HOME", home)

	origWd, _ := os.Getwd()
	_ = os.Chdir(work)
	defer func() { _ = os.Chdir(origWd) }()

	// Without TTY and without prompt, should refuse TUI
	origTTY := os.Getenv("PROTON_FORCE_TTY")
	defer func() { _ = os.Setenv("PROTON_FORCE_TTY", origTTY) }()
	_ = os.Unsetenv("PROTON_FORCE_TTY")

	err := run(ctx, []string{})
	if err == nil || !strings.Contains(err.Error(), "refusing to start the TUI without a terminal") {
		t.Fatalf("expected terminal refusal error, got: %v", err)
	}

	// Headless prompt execution with direct tool call
	err = run(ctx, []string{"-y", "-p", `/call bash {"command":"echo 'in-process-ok'"}`})
	if err != nil {
		t.Fatalf("run with -p failed: %v", err)
	}

	// Output format JSON
	err = run(ctx, []string{"-y", "--output", "json", "-p", `/call bash {"command":"echo 'json-ok'"}`})
	if err != nil {
		t.Fatalf("run with --output json failed: %v", err)
	}

	// Invalid output format
	err = run(ctx, []string{"--output", "invalid_format", "-p", "test"})
	if err == nil || (!strings.Contains(err.Error(), "unsupported output format") && !strings.Contains(err.Error(), "unknown output format")) {
		t.Fatalf("expected unsupported output format error, got: %v", err)
	}

	// Agent profiles (pow, dex, int, worker, explorer, reviewer)
	for _, profile := range []string{"pow", "int", "dex"} {
		err = run(ctx, []string{"-y", "--agent", profile, "-p", `/call bash {"command":"echo 'profile-ok'"}`})
		if err != nil {
			t.Fatalf("run with --agent %s failed: %v", profile, err)
		}
	}

	// Environment variable overrides: PROTON_SANDBOX, PROTON_SESSION_ID, PROTON_TRUST_PROJECT
	origSandbox := os.Getenv("PROTON_SANDBOX")
	origSessionID := os.Getenv("PROTON_SESSION_ID")
	origTrust := os.Getenv("PROTON_TRUST_PROJECT")
	defer func() {
		_ = os.Setenv("PROTON_SANDBOX", origSandbox)
		_ = os.Setenv("PROTON_SESSION_ID", origSessionID)
		_ = os.Setenv("PROTON_TRUST_PROJECT", origTrust)
	}()

	_ = os.Setenv("PROTON_SANDBOX", "workspace")
	_ = os.Setenv("PROTON_SESSION_ID", "env-session-id-456")
	_ = os.Setenv("PROTON_TRUST_PROJECT", "1")
	err = run(ctx, []string{"-y", "-p", `/call bash {"command":"echo 'env-ok'"}`})
	if err != nil {
		t.Fatalf("run with env overrides failed: %v", err)
	}

	// Resume latest session without explicit ID
	err = run(ctx, []string{"-y", "-r", "-p", `/call bash {"command":"echo 'resumed-latest'"}`})
	if err != nil {
		t.Fatalf("run with -r (resume latest) failed: %v", err)
	}

	// Resume non-existent explicit session
	err = run(ctx, []string{"-y", "-r", "-s", "non-existent-session-id-999", "-p", "test"})
	if err == nil || !strings.Contains(err.Error(), "not found to resume") {
		t.Fatalf("expected session not found error, got: %v", err)
	}

	// ACP flag with already-cancelled context so Serve exits immediately
	acpCtx, acpCancel := context.WithCancel(ctx)
	acpCancel()
	_ = run(acpCtx, []string{"--acp"})
}

func TestMainHelpers(t *testing.T) {
	// truthy
	if !truthy("1") || !truthy("true") || !truthy("yes") || !truthy("on") || !truthy("TRUE") {
		t.Error("truthy returned false for true values")
	}
	if truthy("0") || truthy("false") || truthy("no") || truthy("off") || truthy("") {
		t.Error("truthy returned true for false values")
	}

	// workspaceKey
	k1 := workspaceKey("/path/to/project")
	k2 := workspaceKey("/path/to/project")
	k3 := workspaceKey("/path/to/other")
	if k1 != k2 {
		t.Errorf("workspaceKey not deterministic: %s != %s", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("workspaceKey collision for different paths: %s == %s", k1, k3)
	}

	// generateSessionID
	sid := generateSessionID("/test/dir")
	if !strings.HasPrefix(sid, "workspace-") {
		t.Errorf("unexpected sessionID format: %s", sid)
	}

}

func TestMainConfiguredTelemetryObserver(t *testing.T) {
	orig := os.Getenv("PROTON_TELEMETRY")
	defer func() { _ = os.Setenv("PROTON_TELEMETRY", orig) }()

	_ = os.Setenv("PROTON_TELEMETRY", "off")
	obs, err := configuredTelemetryObserver()
	if err != nil || obs != nil {
		t.Errorf("expected nil observer for off, got err=%v, obs=%v", err, obs)
	}

	_ = os.Setenv("PROTON_TELEMETRY", "stderr")
	obs, err = configuredTelemetryObserver()
	if err != nil || obs == nil {
		t.Errorf("expected non-nil observer for stderr, got err=%v, obs=%v", err, obs)
	}
}
