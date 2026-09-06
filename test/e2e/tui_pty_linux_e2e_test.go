//go:build linux

package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestE2ETUIStartupAndExitWithRealPTY(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	master, slave := openLinuxPTY(t, 120, 40)
	defer master.Close()
	defer slave.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, protonBin)
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PROTON_HOME="+home, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start Proton on PTY: %v", err)
	}
	_ = slave.Close()

	var output bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&output, master)
		close(readDone)
	}()

	waitForPTYOutput(t, &output, 2*time.Second)
	if _, err := master.Write([]byte{3}); err != nil {
		t.Fatalf("send Ctrl+C to PTY: %v", err)
	}
	if err := cmd.Wait(); err != nil && ctx.Err() != nil {
		t.Fatalf("TUI did not exit before timeout: %v", err)
	}
	_ = master.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
	}

	view := output.String()
	if !strings.Contains(view, "Proton") && !strings.Contains(view, "\x1b[") {
		t.Fatalf("PTY did not receive TUI output: %q", view)
	}
}

func openLinuxPTY(t *testing.T, cols, rows uint16) (*os.File, *os.File) {
	t.Helper()
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	if err := unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, 0); err != nil {
		_ = unix.Close(masterFD)
		t.Fatalf("unlock PTY: %v", err)
	}
	ptyNumber, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	if err != nil {
		_ = unix.Close(masterFD)
		t.Fatalf("resolve PTY number: %v", err)
	}
	slavePath := fmt.Sprintf("/dev/pts/%d", ptyNumber)
	slaveFD, err := unix.Open(slavePath, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Close(masterFD)
		t.Fatalf("open PTY slave: %v", err)
	}
	if err := unix.IoctlSetWinsize(slaveFD, unix.TIOCSWINSZ, &unix.Winsize{Col: cols, Row: rows}); err != nil {
		_ = unix.Close(slaveFD)
		_ = unix.Close(masterFD)
		t.Fatalf("set PTY size: %v", err)
	}
	return os.NewFile(uintptr(masterFD), "ptmx"), os.NewFile(uintptr(slaveFD), slavePath)
}

func waitForPTYOutput(t *testing.T, output *bytes.Buffer, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if output.Len() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for TUI output on PTY")
}
