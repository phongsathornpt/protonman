//go:build desktop || desktop_gio

package shell

import (
	"testing"

	"gioui.org/layout"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestRuntimePopoverDisabledWhenBusy(t *testing.T) {
	sh := New(NewTheme("light"), Bindings{})
	var modelChanged, reasoningChanged bool
	sh.bind.SetRuntimeModel = func(_, _ string) { modelChanged = true }
	sh.bind.SetRuntimeReasoning = func(_ string) { reasoningChanged = true }

	session := desktopstate.SessionState{
		ID:              "sess-1",
		AvailableModels: []string{"claude-3-7-sonnet"},
		Runtime: desktopstate.RuntimeSettingsState{
			Provider:  "protonman",
			Model:     "claude-3-5-sonnet",
			Reasoning: "auto",
		},
	}
	snapshot := controller.Snapshot{
		ActiveAgentID: "protonman",
	}

	gtx := testLayoutContext()

	// Try clicking model button when enabled is false
	sh.agentModelButton("claude-3-7-sonnet").Click()
	sh.layoutModelPopover(gtx, session, snapshot, false)
	if modelChanged {
		t.Fatal("model should not change when popover is disabled")
	}

	// Try clicking reasoning button when enabled is false
	sh.popoverReasoningButton("high").Click()
	sh.layoutReasoningPopover(gtx, session, false)
	if reasoningChanged {
		t.Fatal("reasoning should not change when popover is disabled")
	}
}

func TestPermissionModeDisabledWhenBusy(t *testing.T) {
	sh := New(NewTheme("light"), Bindings{})
	var modeChanged bool
	sh.bind.SetRuntimePermissionMode = func(_ string) { modeChanged = true }

	session := desktopstate.SessionState{
		ID: "sess-1",
		Runtime: desktopstate.RuntimeSettingsState{
			PermissionMode: "ask",
		},
	}

	gtx := testLayoutContext()
	sh.popoverPermissionModeButton("plan").Click()
	sh.layoutPermissionModePopover(gtx, session, false)

	if modeChanged {
		t.Fatal("permission mode should not change when popover is disabled")
	}
}

// Only one runtime popover may be open at a time. Overlapping overlays make the
// composer unusable and let a click land on the wrong surface, so opening one
// popover must close the others and ClosePopovers must close all of them.
func TestRuntimePopoverMutualExclusivity(t *testing.T) {
	sh := New(NewTheme("light"), Bindings{})
	widgets := sh.runtimeComponent.Widgets()
	if widgets.ModelPopoverVisible || widgets.ReasoningPopoverVisible || widgets.PermissionModePopoverVisible {
		t.Fatal("runtime popovers must start closed")
	}

	sh.openModelPopover()
	if !widgets.ModelPopoverVisible || widgets.ReasoningPopoverVisible || widgets.PermissionModePopoverVisible {
		t.Fatalf("after openModelPopover: model=%v reasoning=%v permission=%v; want only model open",
			widgets.ModelPopoverVisible, widgets.ReasoningPopoverVisible, widgets.PermissionModePopoverVisible)
	}
	if !widgets.ModelSearchFocusPending {
		t.Fatal("opening the model popover must request search focus")
	}

	sh.openReasoningPopover()
	if widgets.ModelPopoverVisible || !widgets.ReasoningPopoverVisible || widgets.PermissionModePopoverVisible {
		t.Fatalf("after openReasoningPopover: model=%v reasoning=%v permission=%v; want only reasoning open",
			widgets.ModelPopoverVisible, widgets.ReasoningPopoverVisible, widgets.PermissionModePopoverVisible)
	}

	sh.openPermissionModePopover()
	if widgets.ModelPopoverVisible || widgets.ReasoningPopoverVisible || !widgets.PermissionModePopoverVisible {
		t.Fatalf("after openPermissionModePopover: model=%v reasoning=%v permission=%v; want only permission open",
			widgets.ModelPopoverVisible, widgets.ReasoningPopoverVisible, widgets.PermissionModePopoverVisible)
	}

	sh.closePopovers()
	if widgets.ModelPopoverVisible || widgets.ReasoningPopoverVisible || widgets.PermissionModePopoverVisible {
		t.Fatalf("closePopovers left model=%v reasoning=%v permission=%v open",
			widgets.ModelPopoverVisible, widgets.ReasoningPopoverVisible, widgets.PermissionModePopoverVisible)
	}
}

// Opening an overlay from the live tail must suspend tail-follow, and closing
// it must restore the tail and clear the anchor. If the anchor leaked, the
// transcript would stop following new messages forever after the first popover.
func TestOpeningPopoverSuspendsTailFollowUntilClosed(t *testing.T) {
	sh := New(NewTheme("light"), Bindings{})
	timeline := sh.conversationUI.Timeline()

	sh.openModelPopover()
	if !sh.tailFollowBeforeOverlay {
		t.Fatal("opening an overlay from the live tail must suspend tail-follow")
	}
	// move the transcript while suspended; closing must snap back to the end.
	timeline.Position = layout.Position{First: 12, Offset: 400}
	timeline.ScrollToEnd = false

	sh.closePopovers()
	if sh.tailFollowBeforeOverlay {
		t.Fatal("closing the overlay must clear the tail-follow anchor")
	}
	if !timeline.ScrollToEnd || timeline.Position.First != 0 {
		t.Fatalf("closing an overlay must resume the tail: ScrollToEnd=%v Position=%v",
			timeline.ScrollToEnd, timeline.Position)
	}
}
