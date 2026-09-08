package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	protonBin string
	coverDir  string
)

func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "proton-e2e-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir for binary: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	binPath := filepath.Join(tempDir, "proton")
	// Build the proton binary from repo root.
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find repo root: %v\n", err)
		os.Exit(1)
	}

	coverDir = os.Getenv("PROTON_COVERDIR")
	if coverDir == "" {
		coverDir = filepath.Join(tempDir, "coverdata")
	}
	_ = os.MkdirAll(coverDir, 0o755)

	buildCmd := exec.Command("go", "build", "-cover", "-o", binPath, "./cmd/protonman")
	buildCmd.Dir = repoRoot
	buildCmd.Env = os.Environ()
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build proton binary: %v\n%s\n", err, output)
		os.Exit(1)
	}

	protonBin = binPath
	code := m.Run()

	if os.Getenv("PROTON_E2E_COVERAGE") == "1" {
		percentCmd := exec.Command("go", "tool", "covdata", "percent", "-i="+coverDir)
		if out, err := percentCmd.CombinedOutput(); err == nil && len(out) > 0 {
			fmt.Println("\n=== E2E Subprocess Coverage Summary ===")
			fmt.Print(string(out))
		}
	}

	os.Exit(code)
}

type runOptions struct {
	args    []string
	env     []string
	dir     string
	stdin   string
	timeout time.Duration
}

type runResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func runProton(t *testing.T, opts runOptions) runResult {
	t.Helper()

	timeout := opts.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, protonBin, opts.args...)
	if opts.dir != "" {
		cmd.Dir = opts.dir
	}

	cmd.Env = os.Environ()
	if coverDir != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+coverDir)
	}
	if len(opts.env) > 0 {
		cmd.Env = append(cmd.Env, opts.env...)
	}

	if opts.stdin != "" {
		cmd.Stdin = strings.NewReader(opts.stdin)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ok := errorAs(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return runResult{
		stdout:   stdoutBuf.String(),
		stderr:   stderrBuf.String(),
		exitCode: exitCode,
		err:      err,
	}
}

func errorAs(err error, target **exec.ExitError) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		*target = exitErr
		return true
	}
	return false
}

func newTestWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err == nil {
		dir = canonicalDir
	}

	// Initialize a dummy file
	testFile := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("Hello Coding E2E\nLine 2\n"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	// Initialize git repo so git_status can function
	gitInit := exec.Command("git", "init")
	gitInit.Dir = dir
	_ = gitInit.Run()

	gitConfigUser := exec.Command("git", "config", "user.name", "Protonman Test")
	gitConfigUser.Dir = dir
	_ = gitConfigUser.Run()

	gitConfigEmail := exec.Command("git", "config", "user.email", "test@proton.local")
	gitConfigEmail.Dir = dir
	_ = gitConfigEmail.Run()

	return dir
}

func newTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	canonicalHome, err := filepath.EvalSymlinks(home)
	if err == nil {
		home = canonicalHome
	}

	protonDir := filepath.Join(home, ".proton")
	if err := os.MkdirAll(filepath.Join(protonDir, "sessions"), 0o700); err != nil {
		t.Fatalf("create sessions dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(protonDir, "checkpoints"), 0o700); err != nil {
		t.Fatalf("create checkpoints dir: %v", err)
	}

	return home
}
