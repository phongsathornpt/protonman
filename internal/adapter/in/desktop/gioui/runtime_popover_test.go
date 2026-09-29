//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/phongsathornpt/protonman/internal/app"
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
	snapshot := controllerSnapshot{
		ActiveAgentID: "protonman",
	}

	sh.openModelPopover()
	gtx := testLayoutContext()

	// Simulate clicking the agent model button
	sh.agentModelButton("claude-3-7-sonnet").Click()
	sh.layoutModelPopover(gtx, session, snapshot, true)

	if selectedModel != "claude-3-7-sonnet" || selectedProvider != "protonman" {
		t.Fatalf("expected provider 'protonman' and model 'claude-3-7-sonnet', got provider=%q model=%q", selectedProvider, selectedModel)
	}
	if sh.modelPopoverVisible {
		t.Fatal("expected model popover closed after selecting model")
	}
}

func TestRuntimePopoverActiveAgentSearchFilter(t *testing.T) {
	sh := newShell(newTheme("light"))
	var selectedModel string
	sh.onSetRuntimeModel = func(_, m string) {
		selectedModel = m
	}

	session := desktopstate.SessionState{
		ID:              "sess-1",
		AvailableModels: []string{"claude-3-7-sonnet", "gpt-4o", "gemini-2.5-pro"},
		Runtime: desktopstate.RuntimeSettingsState{
			Provider: "protonman",
			Model:    "claude-3-7-sonnet",
		},
	}
	snapshot := controllerSnapshot{
		ActiveAgentID: "protonman",
	}

	sh.openModelPopover()

	// 1. Search for "gemini"
	sh.modelSearchEditor.SetText("gemini")
	gtx := testLayoutContext()
	sh.layoutModelPopover(gtx, session, snapshot, true)

	// Click filtered gemini model
	sh.agentModelButton("gemini-2.5-pro").Click()
	sh.layoutModelPopover(gtx, session, snapshot, true)

	if selectedModel != "gemini-2.5-pro" {
		t.Fatalf("expected 'gemini-2.5-pro' selected, got %q", selectedModel)
	}

	// 2. Test search clear button
	sh.openModelPopover()
	sh.modelSearchEditor.SetText("nomatch")
	sh.modelSearchClearBtn.Click()
	sh.layoutModelPopover(gtx, session, snapshot, true)

	if sh.modelSearchEditor.Text() != "" {
		t.Fatalf("expected search editor text cleared, got %q", sh.modelSearchEditor.Text())
	}
}

func TestRuntimePopoverActiveAgentEmptyState(t *testing.T) {
	sh := newShell(newTheme("light"))
	refreshCalled := false
	sh.onRefreshRuntime = func() {
		refreshCalled = true
	}

	// Agent advertising no models (e.g. cline or opencode)
	session := desktopstate.SessionState{
		ID:              "sess-1",
		AvailableModels: nil,
		Runtime: desktopstate.RuntimeSettingsState{
			Provider: "cline",
			Model:    "default",
		},
	}
	snapshot := controllerSnapshot{
		ActiveAgentID: "cline",
	}

	sh.openModelPopover()
	gtx := testLayoutContext()

	// Click the refresh button in empty state
	sh.modelRefreshButton.Click()
	sh.layoutModelPopover(gtx, session, snapshot, true)

	if !refreshCalled {
		t.Fatal("expected onRefreshRuntime to be called from empty state refresh button")
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
	snapshot := controllerSnapshot{
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

	snapshot := controllerSnapshot{ActiveAgentID: "protonman"}
	gtx := testLayoutContext()

	// Click model chip -> opens model popover
	sh.modelChipButton.Click()
	sh.layoutComposerContextChips(gtx, session, snapshot, true)
	if !sh.modelPopoverVisible || sh.reasoningPopoverVisible {
		t.Fatalf("expected model popover open, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}

	// Click reasoning chip -> switches to reasoning popover
	sh.reasoningChipButton.Click()
	sh.layoutComposerContextChips(gtx, session, snapshot, true)
	if sh.modelPopoverVisible || !sh.reasoningPopoverVisible {
		t.Fatalf("expected reasoning popover open, got model=%v reasoning=%v", sh.modelPopoverVisible, sh.reasoningPopoverVisible)
	}
}

func TestRuntimePopoverModelListChangesWithACPAgent(t *testing.T) {
	sh := newShell(newTheme("light"))
	var selectedModel string
	sh.onSetRuntimeModel = func(_, m string) {
		selectedModel = m
	}

	// Session 1 belongs to Protonman
	sessionProton := desktopstate.SessionState{
		ID:              "sess-proton",
		AgentID:         "protonman",
		AvailableModels: []string{"claude-3-7-sonnet", "gpt-4o"},
		Runtime: desktopstate.RuntimeSettingsState{
			Model: "claude-3-7-sonnet",
		},
	}
	snapshotProton := controllerSnapshot{
		ActiveAgentID: "protonman",
		AgentProfiles: []app.ACPAgentProfile{
			{ID: "protonman", DisplayName: "Protonman"},
			{ID: "cline", DisplayName: "Cline"},
		},
		AgentAvailableModels: map[string][]string{
			"protonman": {"claude-3-7-sonnet", "gpt-4o"},
			"cline":     {"claude-3-5-sonnet", "deepseek-coder"},
		},
	}

	sh.openModelPopover()
	gtx := testLayoutContext()

	// 1. Layout for Protonman session
	sh.layoutModelPopover(gtx, sessionProton, snapshotProton, true)
	// Verify Protonman models are clickable
	sh.agentModelButton("gpt-4o").Click()
	sh.layoutModelPopover(gtx, sessionProton, snapshotProton, true)
	if selectedModel != "gpt-4o" {
		t.Fatalf("expected gpt-4o from protonman session, got %q", selectedModel)
	}

	// 2. When switching to Cline session
	sessionCline := desktopstate.SessionState{
		ID:              "sess-cline",
		AgentID:         "cline",
		AvailableModels: []string{"claude-3-5-sonnet", "deepseek-coder"},
		Runtime: desktopstate.RuntimeSettingsState{
			Model: "claude-3-5-sonnet",
		},
	}
	snapshotCline := controllerSnapshot{
		ActiveAgentID: "cline",
		AgentProfiles: []app.ACPAgentProfile{
			{ID: "protonman", DisplayName: "Protonman"},
			{ID: "cline", DisplayName: "Cline"},
		},
		AgentAvailableModels: map[string][]string{
			"protonman": {"claude-3-7-sonnet", "gpt-4o"},
			"cline":     {"claude-3-5-sonnet", "deepseek-coder"},
		},
	}

	sh.openModelPopover()
	// Click Cline's model
	sh.agentModelButton("deepseek-coder").Click()
	sh.layoutModelPopover(gtx, sessionCline, snapshotCline, true)
	if selectedModel != "deepseek-coder" {
		t.Fatalf("expected deepseek-coder from cline session, got %q", selectedModel)
	}

	// 3. Fallback when session.AvailableModels is empty but snapshot.AgentAvailableModels has models
	sessionEmptyModels := desktopstate.SessionState{
		ID:      "sess-cline-empty",
		AgentID: "cline",
	}
	sh.openModelPopover()
	sh.agentModelButton("claude-3-5-sonnet").Click()
	sh.layoutModelPopover(gtx, sessionEmptyModels, snapshotCline, true)
	if selectedModel != "claude-3-5-sonnet" {
		t.Fatalf("expected claude-3-5-sonnet from agent available models fallback, got %q", selectedModel)
	}

	if dir := os.Getenv("CAPTURE_ARTIFACTS"); dir != "" {
		captureDesktopWidget(t, filepath.Join(dir, "acp_agent_proton_models.png"), image.Pt(840, 360), sh, func(gtx layout.Context) layout.Dimensions {
			sh.modelPopoverVisible = true
			return sh.layoutModelPopover(gtx, sessionProton, snapshotProton, true)
		})
		captureDesktopWidget(t, filepath.Join(dir, "acp_agent_cline_models.png"), image.Pt(840, 360), sh, func(gtx layout.Context) layout.Dimensions {
			sh.modelPopoverVisible = true
			return sh.layoutModelPopover(gtx, sessionCline, snapshotCline, true)
		})
	}
}

func TestFreeModelDetectionAndGrouping(t *testing.T) {
	// 1. Detection tests
	freeCases := []struct {
		modelID string
		want    bool
	}{
		{"cline/google/gemma-4-26b-a4b-it:free", true},
		{"cline/qwen/qwen3.8-27b:free", true},
		{"cline/openrouter/free", true},
		{"openrouter/free", true},
		{"cline/meta/muse-spark-1.3-contributor", true},
		{"cline/deepseek/deepseek-v4.1-flash", true},
		{"cline/xiaomi/mimo-v2.6-flash", true},
		{"cline-free/mimo-v2.6-flash", true},
		{"MiMo-V2.6-Flash (free)", true},
		{"DeepSeek V4.1 Flash (free)", true},
		{"Muse Spark 1.3 Contributor (free)", true},
		{"stealth/pixel-canary", true},
		{"stealth/space-bunny-alpha", true},
		{"big-pickle", true},
		{"deepseek-coder", false},
		{"proton/glm-5.3-flash", false},
		{"proton/deepseek-v4.1-flash", false},
		{"proton/muse-spark-1.3-contributor", false},
		{"protonman/deepseek-v4.1-flash", false},
		{"claude-3-7-sonnet", false},
	}
	for _, tc := range freeCases {
		if got := isFreeModel(tc.modelID); got != tc.want {
			t.Errorf("isFreeModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}

	// Provider & agent checks
	if !isProtonmanAgent("protonman") || !isProtonmanAgent("proton") || !isProtonmanAgent("") {
		t.Fatal("expected isProtonmanAgent to be true for protonman, proton, and empty")
	}
	if isProtonmanAgent("cline") {
		t.Fatal("expected isProtonmanAgent to be false for cline")
	}
	if !isClineProvider("cline", "", nil, nil) {
		t.Fatal("expected isClineProvider to be true for cline agent")
	}
	if isClineProvider("protonman", "", nil, nil) || isClineProvider("proton", "", nil, nil) || isClineProvider("", "", nil, nil) {
		t.Fatal("expected isClineProvider to be false for protonman")
	}

	// 2. Partition tests
	allModels := []string{
		"cline/anthropic/claude-3-5-sonnet",
		"cline/google/gemma-4-26b-a4b-it:free",
		"cline/deepseek/deepseek-chat",
		"cline/qwen/qwen3.8-27b:free",
	}
	free, other := partitionModels(allModels)
	if len(free) != 2 || len(other) != 2 {
		t.Fatalf("expected 2 free and 2 other, got free=%d other=%d", len(free), len(other))
	}
	if !hasAnyFreeModel(allModels) {
		t.Fatal("expected hasAnyFreeModel to be true")
	}

	clineCuratedModels := []string{
		"cline/meta/muse-spark-1.3-contributor",
		"cline/deepseek/deepseek-v4.1-flash",
		"cline/xiaomi/mimo-v2.6-flash",
		"cline/anthropic/claude-3-5-sonnet",
	}
	clineFree, clineOther := partitionModels(clineCuratedModels)
	if len(clineFree) != 3 || len(clineOther) != 1 {
		t.Fatalf("expected 3 free and 1 other for cline curated, got free=%d other=%d", len(clineFree), len(clineOther))
	}

	protonModels := []string{
		"proton/glm-5.3-flash",
		"proton/glm-5.3-flashx",
		"proton/deepseek-v4.1-flash",
		"proton/muse-spark-1.3-contributor",
	}
	if hasAnyFreeModel(protonModels) {
		t.Fatal("expected hasAnyFreeModel to be false for protonModels (protonman has no free models)")
	}

	// 3. UI interaction test with Free Models
	sh := newShell(newTheme("dark"))
	var selectedModel string
	sh.onSetRuntimeModel = func(_, m string) {
		selectedModel = m
	}

	sessionCline := desktopstate.SessionState{
		ID:              "sess-cline",
		AgentID:         "cline",
		AvailableModels: allModels,
	}
	snapshotCline := controllerSnapshot{
		ActiveAgentID: "cline",
		AgentProfiles: []app.ACPAgentProfile{
			{ID: "cline", DisplayName: "Cline"},
		},
	}

	gtx := testLayoutContext()
	sh.openModelPopover()

	// Select a free model
	sh.agentModelButton("cline/google/gemma-4-26b-a4b-it:free").Click()
	sh.layoutModelPopover(gtx, sessionCline, snapshotCline, true)

	if selectedModel != "cline/google/gemma-4-26b-a4b-it:free" {
		t.Fatalf("expected selectedModel to be free model, got %q", selectedModel)
	}
	if sh.modelPopoverVisible {
		t.Fatal("expected popover to close after selection")
	}

	if dir := os.Getenv("CAPTURE_ARTIFACTS"); dir != "" {
		captureDesktopWidget(t, filepath.Join(dir, "desktop_cline_free_grouping_widget.png"), image.Pt(840, 420), sh, func(gtx layout.Context) layout.Dimensions {
			sh.modelPopoverVisible = true
			return sh.layoutModelPopover(gtx, sessionCline, snapshotCline, true)
		})
	}
}

func TestFreeModelSearchFiltering(t *testing.T) {
	sh := newShell(newTheme("dark"))
	allModels := []string{
		"cline/anthropic/claude-3-5-sonnet",
		"cline/google/gemma-4-26b-a4b-it:free",
		"cline/deepseek/deepseek-chat",
		"cline/qwen/qwen3.8-27b:free",
	}
	sessionCline := desktopstate.SessionState{
		ID:              "sess-cline",
		AgentID:         "cline",
		AvailableModels: allModels,
	}
	snapshotCline := controllerSnapshot{
		ActiveAgentID: "cline",
		AgentProfiles: []app.ACPAgentProfile{
			{ID: "cline", DisplayName: "Cline"},
		},
	}

	gtx := testLayoutContext()
	sh.openModelPopover()

	// Search for "free" - should match only free models
	sh.modelSearchEditor.SetText("free")
	dims := sh.layoutModelPopover(gtx, sessionCline, snapshotCline, true)
	if dims.Size.Y <= 0 {
		t.Fatal("expected popover to layout with matching free models")
	}

	// Search for "claude" - should match only other model
	sh.modelSearchEditor.SetText("claude")
	dims = sh.layoutModelPopover(gtx, sessionCline, snapshotCline, true)
	if dims.Size.Y <= 0 {
		t.Fatal("expected popover to layout with matching claude model")
	}
}

func TestProtonmanHasNoFreeModels(t *testing.T) {
	sh := newShell(newTheme("dark"))
	protonModels := []string{
		"proton/glm-5.3-flash",
		"proton/glm-5.3-flashx",
		"proton/deepseek-v4.1-flash",
		"proton/muse-spark-1.3-contributor",
	}
	sessionProton := desktopstate.SessionState{
		ID:              "sess-proton",
		AgentID:         "protonman",
		AvailableModels: protonModels,
	}
	snapshotProton := controllerSnapshot{
		ActiveAgentID: "protonman",
		AgentProfiles: []app.ACPAgentProfile{
			{ID: "protonman", DisplayName: "Protonman"},
		},
	}

	gtx := testLayoutContext()
	sh.openModelPopover()

	dims := sh.layoutModelPopover(gtx, sessionProton, snapshotProton, true)
	if dims.Size.Y <= 0 {
		t.Fatal("expected popover to layout successfully for protonman")
	}

	for _, m := range protonModels {
		if isFreeModel(m) {
			t.Fatalf("expected model %q to not be free for protonman", m)
		}
	}
	if hasAnyFreeModel(protonModels) {
		t.Fatal("expected hasAnyFreeModel to be false for protonman models")
	}
	if isClineProvider(sessionProton.AgentID, "", protonModels, snapshotProton.AgentProfiles) {
		t.Fatal("expected isClineProvider to be false for protonman")
	}

	if dir := os.Getenv("CAPTURE_ARTIFACTS"); dir != "" {
		captureDesktopWidget(t, filepath.Join(dir, "desktop_protonman_flat_widget.png"), image.Pt(840, 420), sh, func(gtx layout.Context) layout.Dimensions {
			sh.modelPopoverVisible = true
			return sh.layoutModelPopover(gtx, sessionProton, snapshotProton, true)
		})
	}
}
