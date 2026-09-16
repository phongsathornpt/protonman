package architecture_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMakefileEnforcesBinaryStripping ensures the Makefile specifies -s -w in
// default link flags so that local development builds and installs do not bloat
// by keeping full unstripped symbols and DWARF debug data.
func TestMakefileEnforcesBinaryStripping(t *testing.T) {
	repoRoot := repositoryRoot(t)
	makefilePath := filepath.Join(repoRoot, "Makefile")
	content, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("failed to read Makefile: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "-s -w") {
		t.Errorf("Makefile does not specify '-s -w' in linker flags, risking bloated unstripped binaries")
	}
	if !strings.Contains(contentStr, "DEFAULT_LDFLAGS ?= -s -w") {
		t.Errorf("Makefile should define 'DEFAULT_LDFLAGS ?= -s -w' to allow optional override while defaulting to stripped")
	}
}

// TestCLIBinarySizeBudget guards against unintended binary bloat caused by
// heavy dependencies, large static assets, or transitive compiler additions.
func TestCLIBinarySizeBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary size budget compilation in short test mode")
	}

	repoRoot := repositoryRoot(t)
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "protonman_budget_test")

	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binPath, "./cmd/protonman")
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build test CLI binary: %v, stderr: %s", err, stderr.String())
	}

	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("failed to stat built CLI binary: %v", err)
	}

	// The stripped CLI binary currently sits around 15-16 MB across macOS and Linux.
	// We budget at 18 MB to prevent accidental inclusion of heavy runtime dependencies
	// (e.g. cloud SDKs, heavy container engines, or un-gated desktop libraries).
	const maxCLIBinaryBytes = 18 * 1024 * 1024
	if info.Size() > maxCLIBinaryBytes {
		t.Fatalf("CLI binary size %d bytes (%.2f MB) exceeds budget of %d bytes (%.2f MB)",
			info.Size(), float64(info.Size())/(1024*1024),
			maxCLIBinaryBytes, float64(maxCLIBinaryBytes)/(1024*1024))
	}
}

// TestDesktopBinarySizeBudget guards against unintended binary bloat in the
// Fyne desktop application binary.
func TestDesktopBinarySizeBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping desktop binary size budget compilation in short test mode")
	}

	repoRoot := repositoryRoot(t)
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "protonman_desktop_budget_test")

	cmd := exec.Command("go", "build", "-tags", "desktop", "-trimpath", "-ldflags=-s -w", "-o", binPath, "./cmd/protonman-desktop")
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Desktop build requires platform Cgo/OpenGL dependencies (X11 on Linux, etc.).
		// Skip if toolchain lacks required Cgo headers in current environment.
		t.Skipf("skipping desktop build size test: %v, stderr: %s", err, stderr.String())
	}

	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("failed to stat built desktop binary: %v", err)
	}

	// The stripped desktop binary currently sits around 23-24 MB on macOS.
	// Budget at 28 MB to prevent runaway static asset or dependency bloat.
	const maxDesktopBinaryBytes = 28 * 1024 * 1024
	if info.Size() > maxDesktopBinaryBytes {
		t.Fatalf("Desktop binary size %d bytes (%.2f MB) exceeds budget of %d bytes (%.2f MB)",
			info.Size(), float64(info.Size())/(1024*1024),
			maxDesktopBinaryBytes, float64(maxDesktopBinaryBytes)/(1024*1024))
	}
}
