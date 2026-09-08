//go:build linux

package e2e_test

import (
	"bytes"
	"context"
	"fmt"
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
	if coverDir != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+coverDir)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start Protonman on PTY: %v", err)
	}
	_ = slave.Close()

	var output bytes.Buffer
	firstOutput := make(chan struct{})
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		signaled := false
		for {
			n, err := master.Read(buf)
			if n > 0 {
				_, _ = output.Write(buf[:n])
				if !signaled {
					close(firstOutput)
					signaled = true
				}
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-firstOutput:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for TUI output on PTY")
	}
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
		t.Fatal("timed out draining PTY output")
	}

	view := output.String()
	if !strings.Contains(view, "Protonman") && !strings.Contains(view, "\x1b[") {
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
