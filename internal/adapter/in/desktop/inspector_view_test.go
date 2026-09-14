//go:build desktop

package desktop

import (
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestCompactTextBoundsLongLabels(t *testing.T) {
	got := compactText("  this is a deliberately long desktop label  ", 12)
	if got != "this is a d…" {
		t.Fatalf("unexpected compact label %q", got)
	}
	if len([]rune(got)) != 12 {
		t.Fatalf("expected 12 runes, got %d", len([]rune(got)))
	}
}

func TestSessionDisplayTitleFallsBackToShortID(t *testing.T) {
	session := desktopstate.SessionState{ID: "1234567890abcdef"}
	if got := sessionDisplayTitle(session); got != "Session 12345678" {
		t.Fatalf("unexpected fallback title %q", got)
	}
}

func TestSessionDisplayMetaKeepsStatusVisible(t *testing.T) {
	session := desktopstate.SessionState{
		WorkspaceName: strings.Repeat("workspace-", 12),
		Status:        desktopstate.TaskRunning,
	}
	got := sessionDisplayMeta(session)
	if !strings.HasSuffix(got, " · "+string(desktopstate.TaskRunning)) {
		t.Fatalf("status disappeared from metadata: %q", got)
	}
	if len([]rune(strings.Split(got, " · ")[0])) > headerMetaMaxRunes {
		t.Fatalf("workspace metadata was not bounded: %q", got)
	}
}
