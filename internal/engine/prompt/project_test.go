package prompt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/workspace"
)

func TestLoadProjectInstructionsUsesOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("base rules"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.override.md"), []byte("override rules"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProjectInstructions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Source: AGENTS.override.md") || !strings.Contains(got, "override rules") || strings.Contains(got, "base rules") {
		t.Fatalf("instructions = %q", got)
	}
}

func TestLoadProjectInstructionsBoundsLargeFiles(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("x", MaxProjectInstructionsBytes+100)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProjectInstructions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "truncated by Protonman") || len(got) > MaxProjectInstructionsBytes+128 {
		t.Fatalf("bounded instructions bytes=%d", len(got))
	}
}

func TestLoadProjectInstructionsMissingIsEmpty(t *testing.T) {
	got, err := LoadProjectInstructions(t.TempDir())
	if err != nil || got != "" {
		t.Fatalf("LoadProjectInstructions() = %q, %v", got, err)
	}
}

func TestLoadProjectInstructionsWithPolicyRejectsSymlinkEscape(t *testing.T) {
	if os.Getenv("PROTONMAN_TEST_SYMLINK") == "" {
		if err := os.Symlink(t.TempDir(), filepath.Join(t.TempDir(), "probe")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	root := t.TempDir()
	outside := t.TempDir()
	secretPath := filepath.Join(outside, "AGENTS.md")
	if err := os.WriteFile(secretPath, []byte("outside instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "AGENTS.md")
	if err := os.Symlink(secretPath, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	policy, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadProjectInstructionsWithPolicy(context.Background(), policy, root)
	if err == nil || got != "" {
		t.Fatalf("LoadProjectInstructionsWithPolicy() = %q, %v; want symlink-escape rejection", got, err)
	}
}

func TestLoadProjectInstructionsWithPolicyReadsThrough(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("policy rules"), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := workspace.New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadProjectInstructionsWithPolicy(context.Background(), policy, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "policy rules") {
		t.Fatalf("instructions = %q", got)
	}
}
