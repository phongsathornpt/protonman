//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestSyncRuntimeEditorsPreservesUserDraftOnTurnEvents(t *testing.T) {
	view := newShell(newTheme("dark"))
	session1 := desktopstate.SessionState{
		ID:    "session-1",
		Title: "Session 1",
		Runtime: desktopstate.RuntimeSettingsState{
			Provider:  "openai",
			Model:     "gpt-4o",
			Reasoning: "auto",
		},
	}
	session2 := desktopstate.SessionState{
		ID:    "session-2",
		Title: "Session 2",
		Runtime: desktopstate.RuntimeSettingsState{
			Provider:  "anthropic",
			Model:     "claude-3-5-sonnet",
			Reasoning: "high",
		},
	}
	state := desktopstate.State{
		ActiveSessionID: "session-1",
		Sessions:        []desktopstate.SessionState{session1, session2},
	}

	// 1. Initial sync loads session-1 values
	view.syncRuntimeEditors(state)
	if view.runtimeProviderEditor.Text() != "openai" || view.runtimeModelEditor.Text() != "gpt-4o" {
		t.Fatalf("unexpected initial values: provider=%q model=%q", view.runtimeProviderEditor.Text(), view.runtimeModelEditor.Text())
	}

	// 2. User edits the fields (in-progress draft)
	view.runtimeProviderEditor.SetText("custom-provider")
	view.runtimeModelEditor.SetText("custom-model")

	// 3. Background event updates session-1 reasoning or turn progress
	session1Updated := session1
	session1Updated.Runtime.Reasoning = "medium"
	stateUpdated := desktopstate.State{
		ActiveSessionID: "session-1",
		Sessions:        []desktopstate.SessionState{session1Updated, session2},
	}
	view.syncRuntimeEditors(stateUpdated)

	// User draft MUST NOT be overwritten by turn events on the same session
	if view.runtimeProviderEditor.Text() != "custom-provider" || view.runtimeModelEditor.Text() != "custom-model" {
		t.Fatalf("draft was overwritten by same-session event: provider=%q model=%q", view.runtimeProviderEditor.Text(), view.runtimeModelEditor.Text())
	}

	// 4. Switching to session-2 should load session-2 values
	stateSwitched := desktopstate.State{
		ActiveSessionID: "session-2",
		Sessions:        []desktopstate.SessionState{session1Updated, session2},
	}
	view.syncRuntimeEditors(stateSwitched)
	if view.runtimeProviderEditor.Text() != "anthropic" || view.runtimeModelEditor.Text() != "claude-3-5-sonnet" {
		t.Fatalf("session-2 values not loaded: provider=%q model=%q", view.runtimeProviderEditor.Text(), view.runtimeModelEditor.Text())
	}
}

func TestSettingsModalOpenCloseAndLayout(t *testing.T) {
	view := newShell(newTheme("dark"))
	snapshot := controllerSnapshot{
		Theme: "slate-dark",
		State: desktopstate.State{
			ActiveSessionID: "session-1",
			Sessions: []desktopstate.SessionState{{
				ID:    "session-1",
				Title: "Test",
			}},
		},
		Status: "Connected",
	}

	if view.settingsModalOpen {
		t.Fatal("settings modal should start closed")
	}

	view.openSettingsModal()
	if !view.settingsModalOpen {
		t.Fatal("openSettingsModal did not set settingsModalOpen")
	}

	for _, tab := range []int{0, 1, 2} {
		view.settingsActiveTab = tab
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size.X != 1180 || dims.Size.Y != 760 {
			t.Fatalf("tab %d: layout size = %v, want 1180x760", tab, dims.Size)
		}
	}

	view.closeSettingsModal()
	if view.settingsModalOpen {
		t.Fatal("closeSettingsModal did not close settings modal")
	}
}

func TestSettingsThemeSelectionCallback(t *testing.T) {
	view := newShell(newTheme("dark"))
	var selectedTheme string
	view.onSetTheme = func(mode string) {
		selectedTheme = mode
	}

	view.openSettingsModal()
	snapshot := controllerSnapshot{Theme: "dark"}

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 800, Y: 600}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	// Render settings general tab
	view.settingsActiveTab = 0
	view.layoutSettingsModal(gtx, snapshot)
	router.Frame(gtx.Ops)

	// Simulate clicking the Slate Light button
	if btn, ok := view.settingsThemeButtons["slate-light"]; ok {
		btn.Click()
		var ops2 op.Ops
		gtx2 := layout.Context{
			Ops:         &ops2,
			Constraints: layout.Exact(image.Point{X: 800, Y: 600}),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		view.layoutSettingsModal(gtx2, snapshot)
		if selectedTheme != "slate-light" {
			t.Fatalf("onSetTheme was not called with slate-light, got %q", selectedTheme)
		}
	} else {
		t.Fatal("slate-light button was not initialized")
	}

	// Simulate clicking the System Default button
	if btn, ok := view.settingsThemeButtons["system"]; ok {
		btn.Click()
		var ops3 op.Ops
		gtx3 := layout.Context{
			Ops:         &ops3,
			Constraints: layout.Exact(image.Point{X: 800, Y: 600}),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		view.layoutSettingsModal(gtx3, snapshot)
		if selectedTheme != "system" {
			t.Fatalf("onSetTheme was not called with system, got %q", selectedTheme)
		}
	} else {
		t.Fatal("system button was not initialized")
	}
}

func TestInspectorProtonmanStreamlinedTabs(t *testing.T) {
	view := newShell(newTheme("dark"))
	session := desktopstate.SessionState{
		ID:      "session-1",
		Title:   "Protonman Session",
		AgentID: controllerAgentID,
		Context: desktopstate.SessionContextState{
			Goal: "Build settings modal",
			Todo: desktopstate.TodoState{
				Revision: 1,
				Items: []desktopstate.TodoItemState{
					{Text: "Step 1", Status: "completed"},
					{Text: "Step 2", Status: "in_progress"},
				},
			},
		},
		Runtime: desktopstate.RuntimeSettingsState{
			Provider:  "protonman",
			Model:     "default",
			Reasoning: "medium",
		},
	}
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: session.ID,
			Sessions:        []desktopstate.SessionState{session},
		},
		Status: "Connected",
	}

	// Protonman session should have 3 panels in Plan tab (Goal, Todo, Runtime)
	view.activeInspectorTab = 0
	count := view.inspectorPanelCount(session)
	if count != 3 {
		t.Fatalf("Plan tab panel count = %d, want 3", count)
	}

	// Render all panels in Plan tab
	for i := 0; i < count; i++ {
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(image.Point{X: 300, Y: 600}),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.layoutInspectorPanel(gtx, session, snapshot, i)
		if dims.Size.X == 0 {
			t.Fatalf("panel %d rendered with 0 width", i)
		}
	}
}
