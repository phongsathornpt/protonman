package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !strings.Contains(got, "truncated by Proton") || len(got) > MaxProjectInstructionsBytes+128 {
		t.Fatalf("bounded instructions bytes=%d", len(got))
	}
}

func TestLoadProjectInstructionsMissingIsEmpty(t *testing.T) {
	got, err := LoadProjectInstructions(t.TempDir())
	if err != nil || got != "" {
		t.Fatalf("LoadProjectInstructions() = %q, %v", got, err)
	}
}
