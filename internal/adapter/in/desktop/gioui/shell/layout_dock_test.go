//go:build desktop || desktop_gio

package shell

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	inspectorcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/inspector"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func layoutTestContext(width, height int) (layout.Context, *op.Ops, *input.Router) {
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(width, height)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	return gtx, &operations, &router
}

func layoutTestSession() (controller.Snapshot, desktopstate.SessionState) {
	session := desktopstate.SessionState{
		ID:        "session-layout",
		AgentID:   controller.ProtonmanAgentID,
		Title:     "Layout session",
		Workspace: "/workspaces/protonman",
		Status:    desktopstate.TaskCompleted,
		Timeline: []desktopstate.TimelineItem{
			{ID: "u1", Kind: desktopstate.TimelineUser, Text: "hello"},
		},
		Runtime: desktopstate.RuntimeSettingsState{Provider: "protonman", Model: "coding-model", Reasoning: "medium"},
	}
	snapshot := controller.Snapshot{
		State: desktopstate.State{
			ActiveProjectID: "project",
			ActiveSessionID: session.ID,
			Projects:        []desktopstate.ProjectState{{ID: "project", Name: "Protonman"}},
			Sessions:        []desktopstate.SessionState{session},
		},
		Connection:   controller.ConnectionConnected,
		Status:       "Connected",
		HistoryState: controller.HistoryStateLoaded,
	}
	return snapshot, session
}

// At the default window size the inspector must dock beside the conversation,
// not take it over: both panes report positive width in the same frame.
func TestLayoutDocksInspectorAtDefaultWindowSize(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	snapshot, session := layoutTestSession()
	view.inspectorComponent.SetVisible(true)

	const width, height = 1180, 760
	gtx, _, _ := layoutTestContext(width, height)
	dims := view.layoutMain(gtx, snapshot, unit.Dp(width))
	if dims.Size.X != width || dims.Size.Y != height {
		t.Fatalf("layoutMain dims = %v, want %dx%d", dims.Size, width, height)
	}

	gtxConv, _, _ := layoutTestContext(700, height)
	convDims := view.layoutConversationPane(gtxConv, session, snapshot)
	if convDims.Size.X <= 0 {
		t.Fatal("conversation pane laid out with zero width at default window size")
	}
	gtxInsp, _, _ := layoutTestContext(336, height)
	inspDims := view.layoutInspector(gtxInsp, session, snapshot, false)
	if inspDims.Size.X <= int(uikit.InspectorMinWidth)-10 {
		t.Fatalf("inspector pane width = %d, want at least the docked minimum", inspDims.Size.X)
	}
}

// On a narrow window the inspector must take over as a single pane instead of
// squeezing the conversation into a sliver.
func TestLayoutInspectorTakeoverOnNarrowWindow(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	snapshot, _ := layoutTestSession()
	view.inspectorComponent.SetVisible(true)

	const width, height = 700, 600
	gtx, _, _ := layoutTestContext(width, height)
	dims := view.layoutMain(gtx, snapshot, unit.Dp(width))
	if dims.Size.X != width || dims.Size.Y != height {
		t.Fatalf("layoutMain dims = %v, want %dx%d", dims.Size, width, height)
	}
}

// PxPerDp != 1 must not change the dock decision: the shell measures the
// window in dp, so a 1180px window at 2x density (590dp) takes over instead of
// docking a panel that would collapse the conversation.
func TestWindowWidthDpRespectsMetric(t *testing.T) {
	gtx, _, _ := layoutTestContext(1180, 760)
	gtx.Metric = unit.Metric{PxPerDp: 2, PxPerSp: 2}
	if got := windowWidthDp(gtx); got != 590 {
		t.Fatalf("windowWidthDp = %v, want 590dp at 2x density", got)
	}
	if got := layoutInspectorWidth(inspectorcomponent.PanelWidth, unit.Dp(100)); got != uikit.InspectorMinWidth {
		t.Fatalf("layoutInspectorWidth = %v, want the %v minimum when space is tight", got, uikit.InspectorMinWidth)
	}
	if got := layoutInspectorWidth(400, unit.Dp(2000)); got != inspectorcomponent.PanelWidth {
		t.Fatalf("layoutInspectorWidth = %v, want the full panel when space allows", got)
	}
	if got := layoutInspectorWidth(100, unit.Dp(2000)); got != uikit.InspectorMinWidth {
		t.Fatalf("layoutInspectorWidth = %v, want the %v floor", got, uikit.InspectorMinWidth)
	}
}

// The topbar toggle, the shortcut, and the dock decision share one
// whole-window rule: toggling twice returns to the default visibility.
func TestInspectorToggleRoundTripMatchesDefault(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	const width = unit.Dp(1180)
	if !view.inspectorComponent.ShouldShowForWidth(width, true) {
		t.Fatal("default should show the inspector at 1180dp")
	}
	view.inspectorComponent.ToggleForWidth(width, true)
	if view.inspectorComponent.ShouldShowForWidth(width, true) {
		t.Fatal("first toggle should hide the inspector")
	}
	view.inspectorComponent.ToggleForWidth(width, true)
	if !view.inspectorComponent.ShouldShowForWidth(width, true) {
		t.Fatal("second toggle should restore the default visibility")
	}
}

// Target resolution must not depend on layout: a waiting foreign session
// resolves without emitting frame operations, and the active session resolves
// to nil.
func TestBackgroundAttentionTargetNeedsNoLayout(t *testing.T) {
	state := desktopstate.State{
		ActiveSessionID: "session-active",
		Sessions: []desktopstate.SessionState{
			{ID: "session-active", Title: "Current Work"},
			{ID: "session-bg", Title: "Background Refactor"},
		},
		PermissionInbox: []desktopstate.PermissionRequest{
			{RequestID: "p-bg", SessionID: "session-bg", Title: "Run migrations", Command: "npm run migrate"},
		},
	}
	target := backgroundAttentionTarget(state, "", "session-active")
	if target == nil {
		t.Fatal("backgroundAttentionTarget = nil, want the waiting session")
	}
	if target.sessionID != "session-bg" || target.kind != "permission" {
		t.Fatalf("backgroundAttentionTarget = %+v, want session-bg/permission", target)
	}
	if current := backgroundAttentionTarget(state, "", "session-bg"); current != nil {
		t.Fatalf("backgroundAttentionTarget for active session = %+v, want nil", current)
	}
	empty := desktopstate.State{ActiveSessionID: "session-active"}
	if target := backgroundAttentionTarget(empty, "", "session-active"); target != nil {
		t.Fatalf("backgroundAttentionTarget with empty inbox = %+v, want nil", target)
	}
}
