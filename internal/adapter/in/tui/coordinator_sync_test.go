package tui

import (
	"context"
	"testing"

	"github.com/phongsathornpt/proton/internal/adapter/out/config"
	"github.com/phongsathornpt/proton/internal/app"
	"github.com/phongsathornpt/proton/internal/core/permission"
	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/core/workspace"
	"github.com/phongsathornpt/proton/internal/feature/agent"
)

func TestTUI_WithCoordinatorOption(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	ws, err := workspace.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	coordinator := agent.NewCoordinator(nil, registry, ws, policy)
	defer func() { _ = coordinator.Close() }()

	ui, err := NewBubbleTea(
		service,
		registry,
		nil,
		WithCoordinator(coordinator),
	)
	if err != nil {
		t.Fatalf("NewBubbleTea() error = %v", err)
	}
	if !ui.agents.Available() {
		t.Fatal("expected agent service to be available on BubbleTeaUI")
	}
}

func TestTUI_CycleModeUpdatesCoordinator(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	ws, err := workspace.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	coordinator := agent.NewCoordinator(nil, model.registry, ws, policy)
	defer func() { _ = coordinator.Close() }()
	model.agents = app.NewAgents(coordinator)

	// Initially in ModeAsk, planMode = false
	if model.planMode {
		t.Fatal("expected planMode initially false")
	}

	// 1. Cycle to plan mode
	model.cycleMode()
	if !model.planMode {
		t.Fatal("expected planMode to be true after first cycle")
	}
	guard := coordinator.CallGuard()
	if guard == nil {
		t.Fatal("expected coordinator call guard to be set in plan mode")
	}
	// Verify guard blocks mutation
	err = guard(context.Background(), permission.Request{ToolKind: permission.ToolBash, ToolName: "bash"})
	if err == nil {
		t.Fatal("expected plan mode guard to block bash execution")
	}
	// Verify guard allows read
	err = guard(context.Background(), permission.Request{ToolKind: permission.ToolRead, ToolName: "read_file"})
	if err != nil {
		t.Fatalf("expected plan mode guard to allow read_file, got: %v", err)
	}

	// 2. Cycle from plan mode to always-approve
	model.cycleMode()
	if model.planMode {
		t.Fatal("expected planMode to be false after second cycle")
	}
	if coordinator.CallGuard() != nil {
		t.Fatal("expected coordinator call guard to be cleared")
	}
	if coordinator.PermissionMode() != permission.ModeAlwaysApprove {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAlwaysApprove, coordinator.PermissionMode())
	}

	// 3. Cycle from always-approve to ask
	model.cycleMode()
	if coordinator.PermissionMode() != permission.ModeAsk {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAsk, coordinator.PermissionMode())
	}
}

func TestTUI_SlashModeUpdatesCoordinator(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	ws, err := workspace.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	coordinator := agent.NewCoordinator(nil, model.registry, ws, policy)
	defer func() { _ = coordinator.Close() }()
	model.agents = app.NewAgents(coordinator)

	// /mode always-approve
	_ = model.executeCommand("/mode always-approve")
	if coordinator.PermissionMode() != permission.ModeAlwaysApprove {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAlwaysApprove, coordinator.PermissionMode())
	}

	// /mode ask
	_ = model.executeCommand("/mode ask")
	if coordinator.PermissionMode() != permission.ModeAsk {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAsk, coordinator.PermissionMode())
	}

	// /yolo
	_ = model.executeCommand("/yolo")
	if coordinator.PermissionMode() != permission.ModeAlwaysApprove {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAlwaysApprove, coordinator.PermissionMode())
	}

	// /plan on
	_ = model.executeCommand("/plan on")
	if coordinator.CallGuard() == nil {
		t.Fatal("expected coordinator call guard to be set after /plan on")
	}

	// /plan off
	_ = model.executeCommand("/plan off")
	if coordinator.CallGuard() != nil {
		t.Fatal("expected coordinator call guard to be cleared after /plan off")
	}
}

func TestTUI_ReconfigureRunnerUpdatesCoordinatorClient(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	ws, err := workspace.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	coordinator := agent.NewCoordinator(nil, model.registry, ws, policy)
	defer func() { _ = coordinator.Close() }()
	model.agents = app.NewAgents(coordinator)

	if coordinator.LanguageModel() != nil {
		t.Fatal("expected coordinator language model initially nil")
	}

	model.activeModel = "test-model"
	model.activeProvider = "openai"
	model.providers = map[string]config.ProviderConfig{
		"openai": {
			Name:    "openai",
			APIKey:  "sk-test-key",
			BaseURL: "https://api.openai.com/v1",
		},
	}

	model.reconfigureRunner()

	if coordinator.LanguageModel() == nil {
		t.Fatal("expected coordinator language model to be updated after reconfigureRunner()")
	}
}

func TestPlanModeAllowsTaskMetadataButBlocksWorkspaceEdit(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       tool.Kind
		wantDenied bool
	}{
		{name: "update_todo", kind: tool.KindTask, wantDenied: false},
		{name: "write_file", kind: tool.KindEdit, wantDenied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := newNamedTestRegistry(tool.Definition{Name: tc.name, Description: tc.name, Kind: tc.kind, Mutability: tool.MutabilityMutating})
			service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
			m := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "/tmp/proton")
			m.setPlanEnabled(true)
			call, err := tool.NewCall("call", tc.name, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Call(context.Background(), call)
			if tc.wantDenied && err == nil {
				t.Fatal("workspace edit was allowed in plan mode")
			}
			if !tc.wantDenied && err != nil {
				t.Fatalf("task metadata blocked in plan mode: %v", err)
			}
		})
	}
}
