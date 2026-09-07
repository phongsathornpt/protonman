package sandbox

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const requireSandboxIntegrationEnv = "PROTON_REQUIRE_SANDBOX_INTEGRATION"

func TestSandboxIntegrationWorkspaceBoundary(t *testing.T) {
	workspaceDir := integrationTempDir(t)
	outsideDir := integrationTempDir(t)
	insidePath := filepath.Join(workspaceDir, "inside.txt")
	outsidePath := filepath.Join(outsideDir, "outside.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	launcher := integrationLauncher(t, NameWorkspace, workspaceDir)
	command := fmt.Sprintf(
		"printf inside > %s; printf outside > %s",
		shellQuote(insidePath),
		shellQuote(outsidePath),
	)
	cmd, err := launcher.Command(ctx, workspaceDir, command)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("outside write unexpectedly succeeded; output=%q", output)
	}

	inside, err := os.ReadFile(insidePath)
	if err != nil {
		t.Fatalf("workspace write did not succeed: %v", err)
	}
	if got, want := string(inside), "inside"; got != want {
		t.Fatalf("workspace contents = %q, want %q", got, want)
	}
	if _, err := os.Stat(outsidePath); !os.IsNotExist(err) {
		t.Fatalf("outside path was created or stat failed unexpectedly: %v", err)
	}
}

func TestSandboxIntegrationReadOnlyDeniesWorkspaceWrites(t *testing.T) {
	workspaceDir := integrationTempDir(t)
	blockedPath := filepath.Join(workspaceDir, "blocked.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	launcher := integrationLauncher(t, NameReadOnly, workspaceDir)
	cmd, err := launcher.Command(ctx, workspaceDir, "printf blocked > "+shellQuote(blockedPath))
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("read-only write unexpectedly succeeded; output=%q", output)
	}
	if _, err := os.Stat(blockedPath); !os.IsNotExist(err) {
		t.Fatalf("read-only path was created or stat failed unexpectedly: %v", err)
	}
}

func TestSandboxIntegrationRuntimeBinaryAvailable(t *testing.T) {
	workspaceDir := integrationTempDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	launcher := integrationLauncher(t, NameWorkspace, workspaceDir)
	cmd, err := launcher.Command(ctx, workspaceDir, "git --version >/dev/null")
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runtime command failed inside sandbox: %v; output=%s", err, output)
	}
}

func TestSandboxIntegrationStrictBlocksHostNetwork(t *testing.T) {
	workspaceDir := integrationTempDir(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for host network probe: %v", err)
	}
	defer listener.Close()

	preflight, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("host cannot reach network probe listener: %v", err)
	}
	_ = preflight.Close()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	launcher := integrationLauncher(t, NameStrict, workspaceDir)
	command := shellQuote(executable) + " -test.run=^TestSandboxNetworkProbeHelper$ -test.count=1"
	cmd, err := launcher.Command(ctx, workspaceDir, command)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	cmd.Env = append(os.Environ(), "PROTON_SANDBOX_NET_PROBE="+listener.Addr().String())
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("strict sandbox reached host TCP listener; output=%s", output)
	}
	if !strings.Contains(string(output), "network probe blocked") {
		t.Fatalf("network probe failed before exercising the network boundary: %v; output=%s", runErr, output)
	}
}

func TestSandboxNetworkProbeHelper(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("PROTON_SANDBOX_NET_PROBE"))
	if address == "" {
		t.Skip("sandbox network probe helper")
	}
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("network probe blocked: %v", err)
	}
	_ = connection.Close()
}

func integrationLauncher(t *testing.T, name Name, workspaceDir string) *OSLauncher {
	t.Helper()
	requireSandboxExecutable(t)
	profile, err := NewProfile(name, workspaceDir)
	if err != nil {
		t.Fatalf("NewProfile(%s) error = %v", name, err)
	}
	launcher := NewOSLauncher(profile)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe, err := launcher.Command(ctx, workspaceDir, "true")
	if err != nil {
		integrationUnavailable(t, fmt.Sprintf("build %s sandbox probe: %v", name, err))
	}
	if output, err := probe.CombinedOutput(); err != nil {
		integrationUnavailable(t, fmt.Sprintf("start %s sandbox probe: %v; output=%s", name, err, output))
	}
	return launcher
}

func integrationTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve integration temp directory: %v", err)
	}
	return resolved
}

func requireSandboxExecutable(t *testing.T) {
	t.Helper()
	var executable string
	switch runtime.GOOS {
	case "linux":
		executable = "bwrap"
	case "darwin":
		executable = "sandbox-exec"
	default:
		integrationUnavailable(t, "OS sandbox integration is unsupported on "+runtime.GOOS)
		return
	}
	if _, err := exec.LookPath(executable); err != nil {
		integrationUnavailable(t, executable+" is unavailable")
	}
}

func integrationUnavailable(t *testing.T, reason string) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(requireSandboxIntegrationEnv)) == "1" {
		t.Fatalf("required sandbox integration unavailable: %s", reason)
	}
	t.Skip(reason)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
