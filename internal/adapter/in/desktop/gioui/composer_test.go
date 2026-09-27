//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestComposerDraftsStayWithTheirSession(t *testing.T) {
	view := newShell(newTheme("light"))
	view.syncConversation(desktopstate.State{ActiveSessionID: "first"})
	view.composer.SetText("first session draft")

	view.syncConversation(desktopstate.State{ActiveSessionID: "second"})
	if got := view.composer.Text(); got != "" {
		t.Fatalf("new session composer = %q, want empty", got)
	}
	view.composer.SetText("second session draft")

	view.syncConversation(desktopstate.State{ActiveSessionID: "first"})
	if got := view.composer.Text(); got != "first session draft" {
		t.Fatalf("restored first session composer = %q", got)
	}
	view.syncConversation(desktopstate.State{ActiveSessionID: "second"})
	if got := view.composer.Text(); got != "second session draft" {
		t.Fatalf("restored second session composer = %q", got)
	}
}

func TestSubmitComposerTrimsAndClearsDraft(t *testing.T) {
	view := newShell(newTheme("light"))
	view.activeSessionID = "session"
	var submitted []string
	view.onSendPrompt = func(text string) { submitted = append(submitted, text) }
	view.composer.SetText("saved draft")
	view.rememberComposerDraft("session", "older draft")

	view.submitComposer("  send this\n")
	if len(submitted) != 1 || submitted[0] != "send this" {
		t.Fatalf("submitted prompts = %#v, want [send this]", submitted)
	}
	if got := view.composer.Text(); got != "" {
		t.Fatalf("composer after submit = %q, want empty", got)
	}
	if got := view.takeComposerDraft("session"); got != "" {
		t.Fatalf("restored submitted draft = %q, want empty", got)
	}
}

func TestComposerDraftCacheIsBounded(t *testing.T) {
	view := newShell(newTheme("light"))
	for index := 0; index <= maxComposerDrafts; index++ {
		id := fmt.Sprintf("session-%02d", index)
		view.rememberComposerDraft(id, "draft")
	}
	if len(view.composerDrafts) != maxComposerDrafts || len(view.composerDraftOrder) != maxComposerDrafts {
		t.Fatalf("draft cache sizes = %d map entries, %d order entries; want %d", len(view.composerDrafts), len(view.composerDraftOrder), maxComposerDrafts)
	}
	if got := view.takeComposerDraft("session-00"); got != "" {
		t.Fatalf("oldest draft = %q, want evicted", got)
	}
	if got := view.takeComposerDraft(fmt.Sprintf("session-%02d", maxComposerDrafts)); got != "draft" {
		t.Fatalf("newest draft = %q, want retained", got)
	}
}

func TestComposerHelperPreservesImportantStatesWhenCompact(t *testing.T) {
	if got := composerHelper(true, false, false, true); got != "" {
		t.Fatalf("compact ready helper = %q, want hidden", got)
	}
	for _, test := range []struct {
		name      string
		busy      bool
		loading   bool
		connected bool
		want      string
	}{
		{name: "busy", busy: true, connected: true, want: "Use Stop to cancel the active turn"},
		{name: "loading", loading: true, connected: true, want: "Prompting resumes after session history finishes loading"},
		{name: "disconnected", connected: false, want: "Reconnect to the ACP runtime before sending"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := composerHelper(true, test.busy, test.loading, test.connected); got != test.want {
				t.Fatalf("compact helper = %q, want %q", got, test.want)
			}
		})
	}
	if got := composerHelper(false, false, false, true); got != "Enter sends · Shift+Enter adds a line" {
		t.Fatalf("wide ready helper = %q", got)
	}
}

func TestComposerLayoutFitsCompactAndWideConstraints(t *testing.T) {
	snapshot := desktopCaptureSnapshot()
	session := snapshot.State.Sessions[0]
	compactHeight := composerTestLayout(t, 330, 480, session, snapshot).Y
	wideHeight := composerTestLayout(t, 700, 480, session, snapshot).Y
	if compactHeight >= wideHeight {
		t.Fatalf("compact composer height = %d, wide composer height = %d; want compact to use less space", compactHeight, wideHeight)
	}
}

func composerTestLayout(t *testing.T, width, height int, session desktopstate.SessionState, snapshot controllerSnapshot) image.Point {
	t.Helper()
	view := newShell(newTheme("light"))
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, height)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	dims := view.layoutComposer(gtx, session, snapshot)
	if dims.Size.X > width || dims.Size.Y > height {
		t.Fatalf("composer size = %v, exceeds %dx%d constraints", dims.Size, width, height)
	}
	router.Frame(gtx.Ops)
	return dims.Size
}
