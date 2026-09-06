//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCommandsUseIndependentProcessGroups(t *testing.T) {
	profile, err := NewProfile(NameOff, "")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	cmd, err := NewOSLauncher(profile).Command(context.Background(), t.TempDir(), "true")
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("SysProcAttr = %#v, want Setpgid", cmd.SysProcAttr)
	}
}

func TestCancellationKillsShellProcessGroupDescendants(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	profile, err := NewProfile(NameOff, dir)
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := NewOSLauncher(profile).Command(ctx, dir, `sleep 60 & child=$!; printf '%s' "$child" > child.pid; wait`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	pidPath := filepath.Join(dir, "child.pid")
	var childPID int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(pidPath)
		if readErr == nil {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && childPID > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID <= 0 {
		t.Fatal("child pid was not published")
	}
	defer func() { _ = syscall.Kill(childPID, syscall.SIGKILL) }()

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	started := time.Now()
	cancel()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("shell process did not stop within cancellation bound")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}

	goneDeadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(goneDeadline) {
			t.Fatalf("descendant process %d still exists after process-group cancellation (kill0=%v)", childPID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
