//go:build desktop

package desktop

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestSessionNotificationLabelPrefersTitle(t *testing.T) {
	state := desktopstate.State{Sessions: []desktopstate.SessionState{{ID: "abcdef123456", Title: "Fix ACP", WorkspaceName: "protonman"}}}
	if got := sessionNotificationLabelLocked(state, "abcdef123456"); got != "Fix ACP" {
		t.Fatalf("label = %q", got)
	}
}

func TestCompactNotificationTextBoundsErrors(t *testing.T) {
	input := strings.Repeat("failure ", 40)
	got := compactNotificationText(errors.New(input).Error())
	if utf8.RuneCountInString(got) > 160 {
		t.Fatalf("notification text rune count = %d", utf8.RuneCountInString(got))
	}
	if strings.Contains(got, "  ") || strings.Contains(got, "\n") {
		t.Fatalf("notification text not compacted: %q", got)
	}
}
