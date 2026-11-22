//go:build desktop || desktop_gio

package inspector

import "testing"

func TestVisibilityOverrideAndSessionReset(t *testing.T) {
	component := New()
	if !component.ShouldShowForWidth(1180, true) || component.ShouldShowForWidth(900, true) {
		t.Fatal("default visibility should follow the whole-window dock decision, not a single-pane query")
	}
	component.ToggleForWidth(1180, true)
	if component.ShouldShowForWidth(1180, true) {
		t.Fatal("toggle should hide an inspector shown by the dock default")
	}
	component.ResetVisibility()
	if !component.ShouldShowForWidth(1180, true) || component.ShouldShowForWidth(900, true) {
		t.Fatal("reset should clear the override and restore responsive visibility")
	}
}

func TestPanelCountTracksTab(t *testing.T) {
	component := New()
	if got := component.PanelCount(Snapshot{}); got != 3 {
		t.Fatalf("plan panel count = %d, want 3", got)
	}
	component.SetActiveTab(1)
	if got := component.PanelCount(Snapshot{}); got != 1 {
		t.Fatalf("memory panel count = %d, want 1", got)
	}
}

func TestDockedRequiresWholeWindowToFit(t *testing.T) {
	// Default window (1180dp) with sidebar visible:
	// 260 + 1 + 420 + 1 + 280 = 962.
	if !Docked(1180, true) {
		t.Fatal("Docked(1180, sidebar) = false, want true: default window should dock")
	}
	// At 1000dp the panel still fits (260 + 1 + 420 + 1 + 280 = 962),
	// leaving a usable conversation. Below that the inspector takes over
	// instead of squeezing the conversation into a sliver.
	if !Docked(1000, true) {
		t.Fatal("Docked(1000, sidebar) = false, want true: panel plus conversation minimum still fit")
	}
	if Docked(900, true) {
		t.Fatal("Docked(900, sidebar) = true, want false: panel plus conversation minimum do not fit")
	}
	// Sidebar hidden frees its width, so docking fits again sooner.
	if !Docked(760, false) {
		t.Fatal("Docked(760, no sidebar) = false, want true: hiding the sidebar should allow docking")
	}
	// Just below the 962 boundary the inspector takes over: with the sidebar
	// visible the compact sidebar (below 900dp) still needs 922, so any
	// sub-900 window takes over rather than squeezing the conversation.
	if Docked(950, true) {
		t.Fatal("Docked(950, sidebar) = true, want false: 260 + 1 + 420 + 1 + 280 = 962 does not fit")
	}
	if Docked(899, true) {
		t.Fatal("Docked(899, sidebar) = true, want false: compact sidebar still needs 922")
	}
	if !Docked(760, false) {
		t.Fatal("Docked(760, no sidebar) = false, want true: hiding the sidebar frees the panel width")
	}
	if Docked(700, false) {
		t.Fatal("Docked(700, no sidebar) = true, want false: 420 + 280 + 1 = 701 does not fit")
	}
}

func TestShouldShowForWidthMatchesDockedByDefault(t *testing.T) {
	component := New()
	if !component.ShouldShowForWidth(1180, true) {
		t.Fatal("ShouldShowForWidth(1180, sidebar) = false, want true")
	}
	if component.ShouldShowForWidth(900, true) {
		t.Fatal("ShouldShowForWidth(900, sidebar) = true, want false without an explicit override")
	}
}

func TestToggleForWidthAgreesWithDefault(t *testing.T) {
	component := New()
	component.ToggleForWidth(1180, true)
	if component.ShouldShowForWidth(1180, true) {
		t.Fatal("ToggleForWidth on a docked window should hide the inspector")
	}
	component.ToggleForWidth(900, true)
	if !component.ShouldShowForWidth(900, true) {
		t.Fatal("ToggleForWidth on a narrow window should show the inspector as takeover")
	}
}

func TestTabButtonsMatchTabLabels(t *testing.T) {
	component := New()
	if len(component.tabButtons) != len(tabLabels) {
		t.Fatalf("tabButtons has %d slots for %d tabs; a stale slot would swallow clicks", len(component.tabButtons), len(tabLabels))
	}
}
