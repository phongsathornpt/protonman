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
	settingscomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/settings"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestSyncRuntimeEditorsPreservesUserDraftOnTurnEvents(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
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
	if view.runtimeComponent.Widgets().RuntimeProviderEditor.Text() != "openai" || view.runtimeComponent.Widgets().RuntimeModelEditor.Text() != "gpt-4o" {
		t.Fatalf("unexpected initial values: provider=%q model=%q", view.runtimeComponent.Widgets().RuntimeProviderEditor.Text(), view.runtimeComponent.Widgets().RuntimeModelEditor.Text())
	}

	// 2. User edits the fields (in-progress draft)
	view.runtimeComponent.Widgets().RuntimeProviderEditor.SetText("custom-provider")
	view.runtimeComponent.Widgets().RuntimeModelEditor.SetText("custom-model")

	// 3. Background event updates session-1 reasoning or turn progress
	session1Updated := session1
	session1Updated.Runtime.Reasoning = "medium"
	stateUpdated := desktopstate.State{
		ActiveSessionID: "session-1",
		Sessions:        []desktopstate.SessionState{session1Updated, session2},
	}
	view.syncRuntimeEditors(stateUpdated)

	// User draft MUST NOT be overwritten by turn events on the same session
	if view.runtimeComponent.Widgets().RuntimeProviderEditor.Text() != "custom-provider" || view.runtimeComponent.Widgets().RuntimeModelEditor.Text() != "custom-model" {
		t.Fatalf("draft was overwritten by same-session event: provider=%q model=%q", view.runtimeComponent.Widgets().RuntimeProviderEditor.Text(), view.runtimeComponent.Widgets().RuntimeModelEditor.Text())
	}

	// 4. Switching to session-2 should load session-2 values
	stateSwitched := desktopstate.State{
		ActiveSessionID: "session-2",
		Sessions:        []desktopstate.SessionState{session1Updated, session2},
	}
	view.syncRuntimeEditors(stateSwitched)
	if view.runtimeComponent.Widgets().RuntimeProviderEditor.Text() != "anthropic" || view.runtimeComponent.Widgets().RuntimeModelEditor.Text() != "claude-3-5-sonnet" {
		t.Fatalf("session-2 values not loaded: provider=%q model=%q", view.runtimeComponent.Widgets().RuntimeProviderEditor.Text(), view.runtimeComponent.Widgets().RuntimeModelEditor.Text())
	}
}

func TestSettingsModalOpenCloseAndLayout(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	snapshot := controller.Snapshot{
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

	if view.settingsComponent.IsOpen() {
		t.Fatal("settings modal should start closed")
	}

	view.openSettingsModal()
	if !view.settingsComponent.IsOpen() {
		t.Fatal("openSettingsModal did not set settingsModalOpen")
	}

	for _, tab := range []int{0, 1, 2, 3} {
		view.settingsComponent.SetActiveTab(tab)
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.Layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size.X != 1180 || dims.Size.Y != 760 {
			t.Fatalf("tab %d: layout size = %v, want 1180x760", tab, dims.Size)
		}
	}

	view.closeSettingsModal()
	if view.settingsComponent.IsOpen() {
		t.Fatal("closeSettingsModal did not close settings modal")
	}
}

func TestInspectorProtonmanStreamlinedTabs(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	session := desktopstate.SessionState{
		ID:      "session-1",
		Title:   "Protonman Session",
		AgentID: controller.ProtonmanAgentID,
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
	snapshot := controller.Snapshot{
		State: desktopstate.State{
			ActiveSessionID: session.ID,
			Sessions:        []desktopstate.SessionState{session},
		},
		Status: "Connected",
	}

	// Protonman session should have 3 panels in Plan tab (Goal, Todo, Runtime)
	view.inspectorComponent.SetActiveTab(0)
	count := view.inspectorComponent.PanelCount(inspectorcomponent.Snapshot{ExternalAgent: !protonmanSession(session)})
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
		dims := view.layoutInspectorPanel(gtx, session, snapshot, 0, i)
		if dims.Size.X == 0 {
			t.Fatalf("panel %d rendered with 0 width", i)
		}
	}
}

// The settings modal covers the window while it is open, so every tab must lay
// out at exactly the window size. A tab reporting a smaller box would
// letterbox the modal and leave a strip of the chat visible behind it.
func TestSettingsModalOwnsTheWindowAcrossEveryTab(t *testing.T) {
	sh := New(NewTheme("dark"), Bindings{})
	snapshot := controller.Snapshot{
		Theme: "slate-dark",
		State: desktopstate.State{
			ActiveSessionID: "session-1",
			Sessions:        []desktopstate.SessionState{{ID: "session-1", Title: "Test"}},
		},
		Status: "Connected",
	}
	if sh.settingsComponent.IsOpen() {
		t.Fatal("settings modal must start closed")
	}
	sh.openSettingsModal()
	if !sh.settingsComponent.IsOpen() {
		t.Fatal("openSettingsModal did not open the modal")
	}
	for _, tab := range []int{
		settingscomponent.GeneralTab,
		settingscomponent.ProvidersTab,
		settingscomponent.MCPTab,
		settingscomponent.AgentsTab,
	} {
		sh.settingsComponent.SetActiveTab(tab)
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &ops,
			Constraints: layout.Exact(image.Pt(1180, 760)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := sh.Layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size.X != 1180 || dims.Size.Y != 760 {
			t.Fatalf("tab %d: layout size = %v, want 1180x760", tab, dims.Size)
		}
	}
	sh.closeSettingsModal()
	if sh.settingsComponent.IsOpen() {
		t.Fatal("closeSettingsModal did not close the settings modal")
	}
}
