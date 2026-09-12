package transcriptutil

import (
	"os"
	"testing"
)

func TestFormatWorkspaceDisplay(t *testing.T) {
	if got := FormatWorkspaceDisplay(""); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if got := FormatWorkspaceDisplay(home); got != "~" {
			t.Fatalf("got %q, want ~", got)
		}
	}
}

func TestDetectGitBranch(t *testing.T) {
	if got := DetectGitBranch(""); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	branch := DetectGitBranch(".")
	if branch == "" {
		t.Fatal("expected non-empty branch in git repository")
	}
}
