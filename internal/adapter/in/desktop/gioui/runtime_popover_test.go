//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestRuntimePopoverMutualExclusivity(t *testing.T) {
	sh := newShell(newTheme("light"))
	if sh.modelPopoverVisible || sh.reasoningPopoverVisible {
		t.Fatal("expected popovers initially closed")
	}

	sh.openModelPopover()
	if !sh.modelPopoverVisible || sh.reasoningPopoverVisible {
		t.Fatalf("expected only model popover open, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}

	sh.openReasoningPopover()
	if sh.modelPopoverVisible || !sh.reasoningPopoverVisible {
		t.Fatalf("expected only reasoning popover open, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}

	sh.closePopovers()
	if sh.modelPopoverVisible || sh.reasoningPopoverVisible {
		t.Fatalf("expected both popovers closed, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}
}

func TestRuntimePopoverShortcuts(t *testing.T) {
	sh := newShell(newTheme("light"))
	refreshCalled := false
	sh.onRefreshRuntime = func() {
		refreshCalled = true
	}

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(1180, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
		Source:      router.Source(),
	}
	snapshot := controllerSnapshot{Connection: connectionConnected}

	// First frame to register key filters
	sh.handleGlobalShortcuts(gtx, snapshot)
	router.Frame(gtx.Ops)

	// 1. Alt+M opens model popover and triggers refresh
	router.Queue(key.Event{Name: "M", Modifiers: key.ModAlt, State: key.Press})
	ops.Reset()
	sh.handleGlobalShortcuts(gtx, snapshot)
	router.Frame(gtx.Ops)
	if !sh.modelPopoverVisible || sh.reasoningPopoverVisible {
		t.Fatalf("expected model popover open after Alt+M, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}
	if !refreshCalled {
		t.Fatal("expected onRefreshRuntime called on Alt+M")
	}

	// 2. Alt+R toggles reasoning popover and closes model popover
	router.Queue(key.Event{Name: "R", Modifiers: key.ModAlt, State: key.Press})
	ops.Reset()
	sh.handleGlobalShortcuts(gtx, snapshot)
	router.Frame(gtx.Ops)
	if sh.modelPopoverVisible || !sh.reasoningPopoverVisible {
		t.Fatalf("expected reasoning popover open after Alt+R, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}

	// 3. Escape closes open popovers
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	ops.Reset()
	sh.handleGlobalShortcuts(gtx, snapshot)
	router.Frame(gtx.Ops)
	if sh.modelPopoverVisible || sh.reasoningPopoverVisible {
		t.Fatalf("expected both popovers closed after Escape, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}
}

func TestRuntimePopoverAgentModelSelection(t *testing.T) {
	sh := newShell(newTheme("light"))
	var selectedProvider, selectedModel string
	sh.onSetRuntimeModel = func(p, m string) {
		selectedProvider = p
		selectedModel = m
	}

	session := desktopstate.SessionState{
		ID:              "sess-1",
		AvailableModels: []string{"claude-3-7-sonnet", "gpt-4o"},
		Runtime: desktopstate.RuntimeSettingsState{
			Provider: "protonman",
			Model:    "claude-3-5-sonnet",
		},
	}

	sh.openModelPopover()
	gtx := testLayoutContext()

	// Simulate clicking the agent model button
	sh.agentModelButton("claude-3-7-sonnet").Click()
	sh.layoutModelPopover(gtx, session, true)

	if selectedModel != "claude-3-7-sonnet" || selectedProvider != "protonman" {
		t.Fatalf("expected provider 'protonman' and model 'claude-3-7-sonnet', got provider=%q model=%q", selectedProvider, selectedModel)
	}
	if sh.modelPopoverVisible {
		t.Fatal("expected model popover closed after selecting model")
	}
	if len(sh.recentModels) == 0 || sh.recentModels[0].Model != "claude-3-7-sonnet" {
		t.Fatalf("expected model added to recentModels, got %#v", sh.recentModels)
	}
}

func TestRuntimePopoverCuratedPresetSelection(t *testing.T) {
	sh := newShell(newTheme("light"))
	var selectedProvider, selectedModel string
	sh.onSetRuntimeModel = func(p, m string) {
		selectedProvider = p
		selectedModel = m
	}

	session := desktopstate.SessionState{
		ID: "sess-1",
		Runtime: desktopstate.RuntimeSettingsState{
			Provider: "protonman",
			Model:    "default",
		},
	}

	sh.openModelPopover()
	gtx := testLayoutContext()

	// Click first curated preset
	preset := curatedModelPresets[0]
	sh.modelPresetButton(preset.Model).Click()
	sh.layoutModelPopover(gtx, session, true)

	if selectedModel != preset.Model || selectedProvider != preset.Provider {
		t.Fatalf("expected provider %q and model %q, got provider=%q model=%q", preset.Provider, preset.Model, selectedProvider, selectedModel)
	}
	if sh.modelPopoverVisible {
		t.Fatal("expected model popover closed after selecting preset")
	}
}

func TestRuntimePopoverCustomModelApply(t *testing.T) {
	sh := newShell(newTheme("light"))
	var selectedProvider, selectedModel string
	sh.onSetRuntimeModel = func(p, m string) {
		selectedProvider = p
		selectedModel = m
	}

	session := desktopstate.SessionState{
		ID: "sess-1",
		Runtime: desktopstate.RuntimeSettingsState{
			Provider: "protonman",
			Model:    "default",
		},
	}

	sh.openModelPopover()
	sh.popoverProviderEditor.SetText("openai")
	sh.popoverModelEditor.SetText("o3-mini")

	gtx := testLayoutContext()
	sh.popoverApplyModelButton.Click()
	sh.layoutModelPopover(gtx, session, true)

	if selectedModel != "o3-mini" || selectedProvider != "openai" {
		t.Fatalf("expected provider 'openai' and model 'o3-mini', got provider=%q model=%q", selectedProvider, selectedModel)
	}
	if sh.modelPopoverVisible {
		t.Fatal("expected model popover closed after applying custom model")
	}
}

func TestRuntimePopoverReasoningSelection(t *testing.T) {
	sh := newShell(newTheme("light"))
	var selectedReasoning string
	sh.onSetRuntimeReasoning = func(r string) {
		selectedReasoning = r
	}

	session := desktopstate.SessionState{
		ID: "sess-1",
		Runtime: desktopstate.RuntimeSettingsState{
			Reasoning: "auto",
		},
	}

	sh.openReasoningPopover()
	gtx := testLayoutContext()

	sh.popoverReasoningButton("high").Click()
	sh.layoutReasoningPopover(gtx, session, true)

	if selectedReasoning != "high" {
		t.Fatalf("expected reasoning 'high', got %q", selectedReasoning)
	}
	if sh.reasoningPopoverVisible {
		t.Fatal("expected reasoning popover closed after selection")
	}
}

func TestRuntimePopoverDisabledWhenBusy(t *testing.T) {
	sh := newShell(newTheme("light"))
	var modelChanged, reasoningChanged bool
	sh.onSetRuntimeModel = func(_, _ string) { modelChanged = true }
	sh.onSetRuntimeReasoning = func(_ string) { reasoningChanged = true }

	session := desktopstate.SessionState{
		ID:              "sess-1",
		AvailableModels: []string{"claude-3-7-sonnet"},
		Runtime: desktopstate.RuntimeSettingsState{
			Provider:  "protonman",
			Model:     "claude-3-5-sonnet",
			Reasoning: "auto",
		},
	}

	gtx := testLayoutContext()

	// Try clicking model button when enabled is false
	sh.agentModelButton("claude-3-7-sonnet").Click()
	sh.layoutModelPopover(gtx, session, false)
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

func TestComposerContextChipsTogglePopovers(t *testing.T) {
	sh := newShell(newTheme("light"))
	session := desktopstate.SessionState{
		ID: "sess-1",
		Runtime: desktopstate.RuntimeSettingsState{
			Provider:  "protonman",
			Model:     "claude-3-7-sonnet",
			Reasoning: "medium",
		},
	}

	gtx := testLayoutContext()

	// Click model chip -> opens model popover
	sh.modelChipButton.Click()
	sh.layoutComposerContextChips(gtx, session, true)
	if !sh.modelPopoverVisible || sh.reasoningPopoverVisible {
		t.Fatalf("expected model popover open, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}

	// Click reasoning chip -> switches to reasoning popover
	sh.reasoningChipButton.Click()
	sh.layoutComposerContextChips(gtx, session, true)
	if sh.modelPopoverVisible || !sh.reasoningPopoverVisible {
		t.Fatalf("expected reasoning popover open, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}
}
