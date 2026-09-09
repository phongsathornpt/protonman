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
	for _, size := range []struct {
		name       string
		cols, rows uint16
	}{
		{name: "narrow", cols: 40, rows: 12},
		{name: "normal", cols: 80, rows: 24},
		{name: "wide", cols: 120, rows: 40},
	} {
		t.Run(size.name, func(t *testing.T) {
			ws := newTestWorkspace(t)
			home := newTestHome(t)
			master, slave := openLinuxPTY(t, size.cols, size.rows)
			defer master.Close()
			defer slave.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, protonBin)
			cmd.Dir = ws
			cmd.Env = append(os.Environ(), "PROTONMAN_HOME="+home, "TERM=xterm-256color")
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
			for _, unwanted := range []string{"Protonman crashed", "panic:"} {
				if strings.Contains(view, unwanted) {
					t.Fatalf("PTY output contains %q at %dx%d: %q", unwanted, size.cols, size.rows, view)
				}
			}
		})
	}
}

func TestE2ETUIRapidResizeWithRealPTY(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	master, slave := openLinuxPTY(t, 80, 24)
	defer master.Close()
	defer slave.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, protonBin)
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PROTONMAN_HOME="+home, "TERM=xterm-256color")
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
		t.Fatal("timed out waiting for initial TUI output")
	}

	for _, size := range []struct{ cols, rows uint16 }{{40, 12}, {120, 40}, {24, 8}, {100, 30}, {60, 16}} {
		if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: size.cols, Row: size.rows}); err != nil {
			t.Fatalf("resize PTY to %dx%d: %v", size.cols, size.rows, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("TUI exited during resize sequence: %v\noutput: %q", err, output.String())
	}
	if _, err := master.Write([]byte{3}); err != nil {
		t.Fatalf("send Ctrl+C after resize sequence: %v", err)
	}
	if err := cmd.Wait(); err != nil && ctx.Err() != nil {
		t.Fatalf("TUI did not exit after resize sequence: %v", err)
	}
	_ = master.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("timed out draining resized PTY output")
	}
	for _, unwanted := range []string{"Protonman crashed", "panic:"} {
		if strings.Contains(output.String(), unwanted) {
			t.Fatalf("PTY resize output contains %q: %q", unwanted, output.String())
		}
	}
}

func TestE2ETUIResizeDuringRunningTool(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	master, slave := openLinuxPTY(t, 80, 24)
	defer master.Close()
	defer slave.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, protonBin, "-y")
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PROTONMAN_HOME="+home, "TERM=xterm-256color")
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
		t.Fatal("timed out waiting for initial TUI output")
	}

	time.Sleep(120 * time.Millisecond)
	command := "!for i in $(seq 1 12); do echo live-$i; sleep 0.04; done"
	if _, err := master.Write([]byte(command)); err != nil {
		t.Fatalf("type long-running tool: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := master.Write([]byte{'\r'}); err != nil {
		t.Fatalf("submit long-running tool: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	for _, size := range []struct{ cols, rows uint16 }{{48, 12}, {110, 32}, {30, 9}, {88, 22}} {
		if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: size.cols, Row: size.rows}); err != nil {
			t.Fatalf("resize PTY during tool to %dx%d: %v", size.cols, size.rows, err)
		}
		time.Sleep(45 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("TUI exited while tool was running: %v\noutput: %q", err, output.String())
	}
	time.Sleep(500 * time.Millisecond)
	view := output.String()
	if !strings.Contains(view, "live-12") {
		t.Fatalf("running tool output missing terminal line after resize: %q", view)
	}
	for _, unwanted := range []string{"Protonman crashed", "panic:"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("running-tool resize output contains %q: %q", unwanted, view)
		}
	}

	_, _ = master.Write([]byte{3})
	if err := cmd.Wait(); err != nil && ctx.Err() != nil {
		t.Fatalf("TUI did not exit after running-tool resize: %v", err)
	}
	_ = master.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("timed out draining running-tool PTY output")
	}
}

func TestE2ETUIBracketedUnicodePasteSurvivesResize(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	master, slave := openLinuxPTY(t, 72, 18)
	defer master.Close()
	defer slave.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, protonBin)
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PROTONMAN_HOME="+home, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start Protonman on PTY: %v", err)
	}
	_ = slave.Close()

	var output bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				_, _ = output.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	paste := "\x1b[200~ภาษาไทย café 東京\nsecond line\nthird line\x1b[201~"
	if _, err := master.Write([]byte(paste)); err != nil {
		t.Fatalf("paste unicode multiline draft: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: 36, Row: 10}); err != nil {
		t.Fatalf("resize PTY after paste: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	view := output.String()
	for _, want := range []string{"ภาษาไทย", "café", "東京", "second line", "third line"} {
		if !strings.Contains(view, want) {
			t.Fatalf("pasted PTY output missing %q: %q", want, view)
		}
	}
	for _, unwanted := range []string{"Protonman crashed", "panic:"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("unicode paste output contains %q: %q", unwanted, view)
		}
	}

	// First Ctrl+C clears the draft; the second exits the now-idle TUI.
	_, _ = master.Write([]byte{3})
	time.Sleep(50 * time.Millisecond)
	_, _ = master.Write([]byte{3})
	if err := cmd.Wait(); err != nil && ctx.Err() != nil {
		t.Fatalf("TUI did not exit after unicode paste: %v", err)
	}
	_ = master.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("timed out draining unicode paste PTY output")
	}
}

func TestE2ETUIExitsWhenPTYDetaches(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	master, slave := openLinuxPTY(t, 80, 24)
	defer slave.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, protonBin)
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PROTONMAN_HOME="+home, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start Protonman on PTY: %v", err)
	}
	_ = slave.Close()

	buf := make([]byte, 4096)
	_ = master.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := master.Read(buf); err != nil {
		t.Fatalf("wait for initial TUI output: %v", err)
	}
	if err := master.Close(); err != nil {
		t.Fatalf("detach PTY master: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("TUI stayed alive after PTY detached")
	}
}

func TestE2ETUISlashHelpWithRealPTY(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	master, slave := openLinuxPTY(t, 80, 24)
	defer master.Close()
	defer slave.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, protonBin)
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PROTONMAN_HOME="+home, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start Protonman on PTY: %v", err)
	}
	_ = slave.Close()

	var output bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				_, _ = output.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	if _, err := master.Write([]byte("/he")); err != nil {
		t.Fatalf("type slash command: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	view := output.String()
	for _, want := range []string{"tab", "accept", "enter", "run"} {
		if !strings.Contains(view, want) {
			t.Fatalf("slash PTY output missing %q: %q", want, view)
		}
	}
	// First Ctrl+C clears the draft; the second exits the idle TUI.
	_, _ = master.Write([]byte{3})
	time.Sleep(50 * time.Millisecond)
	_, _ = master.Write([]byte{3})
	if err := cmd.Wait(); err != nil && ctx.Err() != nil {
		t.Fatalf("TUI did not exit before timeout: %v", err)
	}
	_ = master.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("timed out draining slash PTY output")
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
