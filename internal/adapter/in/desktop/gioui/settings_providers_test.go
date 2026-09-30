//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestSettingsProvidersPanelLayoutAndPresets(t *testing.T) {
	view := newShell(newTheme("dark"))
	view.openSettingsModal()
	view.settingsActiveTab = 1 // Model Providers tab

	snapshot := controllerSnapshot{
		Providers: []desktopstate.ProviderState{
			{ID: "protonman", Name: "Protonman", IsConfigured: true, IsActive: true},
			{ID: "opencode", Name: "OpenCode Free", IsConfigured: true, IsFree: true},
			{ID: "ollama", Name: "Ollama", IsConfigured: true, IsFree: true},
			{ID: "openai", Name: "OpenAI Official", RequiresKey: true, HasKey: true, IsConfigured: true},
			{ID: "anthropic", Name: "Anthropic", RequiresKey: true, HasKey: false, IsConfigured: false},
		},
		ActiveProvider: "protonman",
	}

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
		t.Fatalf("layout size = %v, want 1180x760", dims.Size)
	}

	if view.providerFormVisible {
		t.Fatal("provider form should start hidden")
	}

	// Click "+ Add Custom Provider / Proxy"
	view.providerAddCustomBtn.Click()

	var op2 op.Ops
	gtx2 := layout.Context{
		Ops:         &op2,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(2, 0),
		Source:      router.Source(),
	}
	view.layout(gtx2, snapshot)
	router.Frame(gtx2.Ops)

	if !view.providerFormVisible {
		t.Fatal("provider form should be visible after clicking add custom provider")
	}

	// Form defaults
	if view.providerSelectedType != "openai" {
		t.Fatalf("expected protocol openai, got %q", view.providerSelectedType)
	}
}

func TestSettingsProvidersSaveCustomProviderFlow(t *testing.T) {
	view := newShell(newTheme("dark"))
	view.openSettingsModal()
	view.settingsActiveTab = 1

	snapshot := controllerSnapshot{
		Providers: []desktopstate.ProviderState{
			{ID: "protonman", Name: "Protonman", IsConfigured: true, IsActive: true},
		},
		ActiveProvider: "protonman",
	}

	var savedParams acpProvidersSaveParams
	saveCalled := false
	view.onSaveProvider = func(params acpProvidersSaveParams, onDone func(error)) {
		savedParams = params
		saveCalled = true
		if onDone != nil {
			onDone(nil)
		}
	}

	// Open form
	view.openProviderForm("my-custom-proxy", "My Custom Proxy", "https://proxy.example.com/v1", "openai", "custom-model-1")
	view.providerAPIKeyEditor.SetText("secret-key")

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layout(gtx, snapshot)
	router.Frame(gtx.Ops)

	// Click Save & Activate
	view.providerSaveActivateBtn.Click()

	var op2 op.Ops
	gtx2 := layout.Context{
		Ops:         &op2,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(2, 0),
		Source:      router.Source(),
	}
	view.layout(gtx2, snapshot)
	router.Frame(gtx2.Ops)

	if !saveCalled {
		t.Fatal("onSaveProvider was not called")
	}
	if savedParams.ProviderName != "My Custom Proxy" {
		t.Fatalf("unexpected ProviderName: %q", savedParams.ProviderName)
	}
	if savedParams.BaseURL != "https://proxy.example.com/v1" {
		t.Fatalf("unexpected BaseURL: %q", savedParams.BaseURL)
	}
	if savedParams.APIKey != "secret-key" {
		t.Fatalf("unexpected APIKey: %q", savedParams.APIKey)
	}
	if !savedParams.Activate {
		t.Fatal("expected Activate=true")
	}
	if view.providerFormVisible {
		t.Fatal("provider form should close after successful save")
	}
}

func TestSettingsProvidersTestAndFetchModels(t *testing.T) {
	view := newShell(newTheme("dark"))
	view.openSettingsModal()
	view.settingsActiveTab = 1

	snapshot := controllerSnapshot{
		Providers: []desktopstate.ProviderState{
			{ID: "ollama", Name: "Ollama", BaseURL: "http://localhost:11434/v1"},
		},
	}

	fetchCalled := false
	view.onFetchProviderModels = func(id, baseURL, apiKey, pType string, onDone func([]string, error)) {
		fetchCalled = true
		if onDone != nil {
			onDone([]string{"llama3.3", "qwen2.5-coder"}, nil)
		}
	}

	view.openProviderForm("ollama", "Ollama", "http://localhost:11434/v1", "openai", "")

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layout(gtx, snapshot)
	router.Frame(gtx.Ops)

	// Click Test & Fetch Models
	view.providerTestBtn.Click()

	var op2 op.Ops
	gtx2 := layout.Context{
		Ops:         &op2,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(2, 0),
		Source:      router.Source(),
	}
	view.layout(gtx2, snapshot)
	router.Frame(gtx2.Ops)

	if !fetchCalled {
		t.Fatal("onFetchProviderModels was not called")
	}
	if !strings.Contains(view.providerTestStatus, "2 models found") {
		t.Fatalf("unexpected test status: %q", view.providerTestStatus)
	}
	if view.providerSelectedModel != "llama3.3" {
		t.Fatalf("expected selected model llama3.3, got %q", view.providerSelectedModel)
	}
}

func TestSettingsProvidersDeleteFlow(t *testing.T) {
	view := newShell(newTheme("dark"))
	view.openSettingsModal()
	view.settingsActiveTab = 1

	snapshot := controllerSnapshot{
		Providers: []desktopstate.ProviderState{
			{ID: "custom-one", Name: "Custom One", BaseURL: "https://custom.com/v1"},
		},
	}

	deleteCalled := false
	deletedName := ""
	view.onDeleteProvider = func(name string, onDone func(error)) {
		deleteCalled = true
		deletedName = name
		if onDone != nil {
			onDone(nil)
		}
	}

	view.openProviderForm("custom-one", "Custom One", "https://custom.com/v1", "openai", "")

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layout(gtx, snapshot)
	router.Frame(gtx.Ops)

	// Click Delete Provider
	view.providerDeleteBtn.Click()

	var op2 op.Ops
	gtx2 := layout.Context{
		Ops:         &op2,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(2, 0),
		Source:      router.Source(),
	}
	view.layout(gtx2, snapshot)
	router.Frame(gtx2.Ops)

	if !deleteCalled {
		t.Fatal("onDeleteProvider was not called")
	}
	if deletedName != "Custom One" {
		t.Fatalf("expected deleted provider Custom One, got %q", deletedName)
	}
	if view.providerFormVisible {
		t.Fatal("provider form should close after successful delete")
	}
}
