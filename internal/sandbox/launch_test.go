package sandbox

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOffProfileUsesBareShell(t *testing.T) {
	profile, err := NewProfile(NameOff, "")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	launcher := NewOSLauncher(profile)
	cmd, err := launcher.Command(context.Background(), t.TempDir(), "echo hi")
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if filepath.Base(cmd.Path) == "bwrap" || filepath.Base(cmd.Path) == "sandbox-exec" {
		t.Fatalf("off profile wrapped command: %s", cmd.Path)
	}
}

func TestCommandsConfigureCancellationContainment(t *testing.T) {
	profile, err := NewProfile(NameOff, "")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	cmd, err := NewOSLauncher(profile).Command(context.Background(), t.TempDir(), "true")
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if cmd.WaitDelay != commandWaitDelay {
		t.Fatalf("WaitDelay = %s, want %s", cmd.WaitDelay, commandWaitDelay)
	}
	if cmd.Cancel == nil {
		t.Fatal("Cancel = nil, want process cancellation hook")
	}
}

func TestConfiningProfileFailsClosedWhenToolsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows launcher is fail-closed by design")
	}
	profiles := []Name{
		NameWorkspace,
		NameReadOnly,
		NameStrict,
	}
	for _, name := range profiles {
		t.Run(name.String(), func(t *testing.T) {
			profile, err := NewProfile(name, t.TempDir())
			if err != nil {
				t.Fatalf("NewProfile() error = %v", err)
			}
			launcher := &OSLauncher{
				Profile: profile,
				LookPath: func(string) (string, error) {
					return "", errors.New("missing")
				},
			}
			_, err = launcher.Command(context.Background(), t.TempDir(), "echo hi")
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Command() error = %v, want unavailable", err)
			}
		})
	}
}

func TestSeatbeltProfileDeniesWritesOutsideWorkspace(t *testing.T) {
	profile, err := NewProfile(NameStrict, "/tmp/ws")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	text := seatbeltProfile(profile, "/tmp/ws")
	if !strings.Contains(text, "(deny file-write*") {
		t.Fatalf("seatbelt profile missing write deny: %s", text)
	}
	if !strings.Contains(text, `(require-not (subpath "/tmp/ws"))`) {
		t.Fatalf("seatbelt profile does not exclude workspace from write deny: %s", text)
	}
	if strings.Contains(text, "(allow file-write* (subpath") {
		t.Fatalf("seatbelt profile relies on allow-over-deny semantics: %s", text)
	}
	if !strings.Contains(text, `(require-not (literal "/dev/null"))`) {
		t.Fatalf("seatbelt profile does not preserve /dev/null writes: %s", text)
	}
}

func TestSeatbeltReadOnlyProfileDoesNotExcludeWorkspaceFromWriteDeny(t *testing.T) {
	profile, err := NewProfile(NameReadOnly, "/tmp/ws")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	text := seatbeltProfile(profile, "/tmp/ws")
	if !strings.Contains(text, "(deny file-write*") {
		t.Fatalf("seatbelt profile missing write deny: %s", text)
	}
	if strings.Contains(text, `(require-not (subpath "/tmp/ws"))`) {
		t.Fatalf("read-only profile unexpectedly excludes workspace from deny: %s", text)
	}
	if !strings.Contains(text, `(require-not (literal "/dev/null"))`) {
		t.Fatalf("read-only profile does not preserve /dev/null writes: %s", text)
	}
}

func TestSeatbeltProfileDeniesNetworkWhenRestricted(t *testing.T) {
	profile, err := NewProfile(NameStrict, "/tmp/ws")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	text := seatbeltProfile(profile, "/tmp/ws")
	if !strings.Contains(text, "(deny network*)") {
		t.Fatalf("seatbelt profile missing network deny: %s", text)
	}
	if !strings.Contains(text, "/tmp/ws") {
		t.Fatalf("seatbelt profile missing workspace: %s", text)
	}
}

func TestBwrapExposesHostRuntimeReadOnlyAndWorkspaceWritable(t *testing.T) {
	profile, err := NewProfile(NameWorkspace, "/tmp/ws")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	cmd := bwrapCommand(context.Background(), "bwrap", profile, "/tmp/ws", "/tmp/ws", "true")
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--ro-bind / /") {
		t.Fatalf("bwrap args missing read-only host root: %s", joined)
	}
	if !strings.Contains(joined, "--bind /tmp/ws /tmp/ws") {
		t.Fatalf("bwrap args missing writable workspace bind: %s", joined)
	}
}

func TestBwrapUsesUnshareNetWhenRestricted(t *testing.T) {
	profile, err := NewProfile(NameStrict, "/tmp/ws")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	cmd := bwrapCommand(context.Background(), "bwrap", profile, "/tmp/ws", "/tmp/ws", "true")
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--unshare-net") {
		t.Fatalf("bwrap args missing --unshare-net: %s", joined)
	}
}
