//go:build desktop

package desktop

import (
	"path/filepath"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestResolveNerdFontPathForPrefersOverride(t *testing.T) {
	got := resolveNerdFontPathFor(" /custom/font.ttf ", "/app/protonman-desktop", func(path string) bool {
		return path == "/custom/font.ttf" || path == filepath.Join("/app", "share", "fonts", nerdFontFilename)
	})
	if got != "/custom/font.ttf" {
		t.Fatalf("font path = %q, want override", got)
	}
}

func TestResolveNerdFontPathForUsesBundledAsset(t *testing.T) {
	executable := filepath.Join(string(filepath.Separator), "opt", "protonman", "protonman-desktop")
	want := filepath.Join(filepath.Dir(executable), "share", "fonts", nerdFontFilename)
	got := resolveNerdFontPathFor("", executable, func(path string) bool { return path == want })
	if got != want {
		t.Fatalf("font path = %q, want %q", got, want)
	}
}

func TestTaskStatusIconCoversLifecycle(t *testing.T) {
	statuses := []desktopstate.TaskStatus{
		desktopstate.TaskIdle,
		desktopstate.TaskQueued,
		desktopstate.TaskRunning,
		desktopstate.TaskWaitingPermission,
		desktopstate.TaskWaitingUser,
		desktopstate.TaskPaused,
		desktopstate.TaskCompleted,
		desktopstate.TaskFailed,
	}
	for _, status := range statuses {
		if got := taskStatusIcon(status); got == "" {
			t.Fatalf("status %q has no icon", status)
		}
	}
}
