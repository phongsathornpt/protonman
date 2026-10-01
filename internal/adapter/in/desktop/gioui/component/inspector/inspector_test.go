//go:build desktop || desktop_gio

package inspector

import "testing"

func TestVisibilityOverrideAndSessionReset(t *testing.T) {
	component := New()
	if !component.ShouldShow(true) || component.ShouldShow(false) {
		t.Fatal("default visibility should follow the wide-layout breakpoint")
	}
	component.Toggle(true)
	if component.ShouldShow(true) {
		t.Fatal("toggle should hide an inspector shown by the wide default")
	}
	component.ResetVisibility()
	if !component.ShouldShow(true) || component.ShouldShow(false) {
		t.Fatal("reset should clear the override and restore responsive visibility")
	}
}

func TestPanelCountTracksTabAndAgentType(t *testing.T) {
	component := New()
	if got := component.PanelCount(Snapshot{}); got != 3 {
		t.Fatalf("plan panel count = %d, want 3", got)
	}
	component.SetActiveTab(1)
	if got := component.PanelCount(Snapshot{}); got != 1 {
		t.Fatalf("memory panel count = %d, want 1", got)
	}
	if got := component.PanelCount(Snapshot{ExternalAgent: true}); got != 1 {
		t.Fatalf("external-agent panel count = %d, want 1", got)
	}
}
