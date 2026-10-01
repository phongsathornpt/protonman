//go:build desktop || desktop_gio

package shell

import (
	"image"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestCompactInspectorTextBoundsRuneScanning(t *testing.T) {
	for _, test := range []struct {
		name string
		text string
		max  int
		want string
	}{
		{name: "short text", text: "  short  ", max: 8, want: "short"},
		{name: "truncate", text: "abcdef", max: 4, want: "abc…"},
		{name: "unicode", text: "กขคงจ", max: 4, want: "กขค…"},
		{name: "single rune limit", text: "ab", max: 1, want: "…"},
		{name: "single rune value", text: "ก", max: 1, want: "ก"},
		{name: "nonpositive limit", text: "  keep all  ", max: 0, want: "keep all"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := compactInspectorText(test.text, test.max); got != test.want {
				t.Fatalf("compactInspectorText(%q, %d) = %q, want %q", test.text, test.max, got, test.want)
			}
		})
	}
}

func TestTodoMarkerForStatus(t *testing.T) {
	for status, want := range map[string]string{
		"completed":   "✓",
		"in_progress": "◐",
		"pending":     "○",
	} {
		if got := todoMarkerForStatus(status); got != want {
			t.Fatalf("marker for %q = %q, want %q", status, got, want)
		}
	}
}

func TestCompactInspectorTextIsUTF8Safe(t *testing.T) {
	got := compactInspectorText(strings.Repeat("ก", 20), 8)
	if !utf8.ValidString(got) || len([]rune(got)) != 8 {
		t.Fatalf("compacted text = %q", got)
	}
}

// Each inspector panel must actually render its content. A panel that returns
// empty dimensions still occupies the tab, so it reads as "nothing here" and
// silently hides a projection bug. Goal, todo, memory, runtime, and skills all
// have content in this fixture, so every one of them must produce a box.
func TestInspectorPanelsRenderTheirContent(t *testing.T) {
	sh := New(NewTheme("dark"), Bindings{})
	snapshot := controller.Snapshot{
		State: desktopstate.State{
			ActiveSessionID: "session",
			Sessions: []desktopstate.SessionState{{
				ID:     "session",
				Title:  "Inspector",
				Status: desktopstate.TaskIdle,
				Skills: []desktopstate.SkillState{{Name: "pdf", Description: "parse pdfs", Scope: "project", Active: true, Locked: true, LockStatus: "verified"}},
				Context: desktopstate.SessionContextState{
					Goal: "Ship the inspector slice",
					Todo: desktopstate.TodoState{Revision: 7, Items: []desktopstate.TodoItemState{
						{ID: "todo", Text: "Run focused tests", Status: "in_progress"},
					}},
					Memory: desktopstate.MemoryState{
						WorkspaceKey: "workspace-1",
						Workspace:    []desktopstate.MemoryEntryState{{ID: "m1", Kind: "repo_fact", Key: "test command", Value: "go test ./...", Confidence: 0.9, UsageCount: 2}},
						Global:       []desktopstate.MemoryEntryState{{ID: "m2", Kind: "preference", Key: "style", Value: "concise", Confidence: 1}},
					},
				},
				Runtime: desktopstate.RuntimeSettingsState{Provider: "openai", Model: "gpt", Reasoning: "high", LowConcurrency: "on"},
			}},
		},
		Connection: controller.ConnectionConnected,
		Status:     "Connected",
	}
	session := snapshot.State.Sessions[0]

	for _, compact := range []bool{false, true} {
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &ops,
			Constraints: layout.Exact(image.Pt(320, 600)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		panels := map[string]layout.Dimensions{
			"goal":    sh.layoutGoalPanel(gtx, session),
			"todo":    sh.layoutTodoPanel(gtx, session),
			"memory":  sh.layoutMemoryPanel(gtx, session),
			"skills":  sh.layoutSkillsPanel(gtx, session),
			"runtime": sh.layoutRuntimePanel(gtx, session, snapshot),
		}
		for name, dims := range panels {
			if dims.Size.X <= 0 || dims.Size.Y <= 0 {
				t.Fatalf("compact=%v: %s panel rendered empty dims %v", compact, name, dims)
			}
		}
	}
}
