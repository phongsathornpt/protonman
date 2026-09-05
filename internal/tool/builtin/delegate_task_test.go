package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
)

type mockSubagentRunner struct {
	content string
}

func (m mockSubagentRunner) Run(_ context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
	return turn.Result{
		Message: model.Message{Role: model.RoleAssistant, Content: m.content},
		Rounds:  1,
	}, nil
}

type mockDelegateRunner struct {
	runFunc func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error)
}

func (m *mockDelegateRunner) Run(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, messages, sink)
	}
	return turn.Result{}, nil
}

func TestDelegateTask_Execute(t *testing.T) {
	ctx := context.Background()

	coord := agent.NewCoordinator(
		nil,
		nil,
		nil,
		nil,
		agent.WithRunnerFactory(func(p agent.Profile, tools *toolcall.Service) (turn.Runner, error) {
			return mockSubagentRunner{content: "found 2 occurrences of auth middleware"}, nil
		}),
	)
	defer coord.Close()

	handler := NewDelegateTask(coord)

	t.Run("successful delegation", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"profile": "explorer",
			"task":    "search for auth middleware",
		})
		call, err := tool.NewCall("call-1", "delegate_task", args)
		if err != nil {
			t.Fatal(err)
		}

		res, err := handler.Execute(ctx, call)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Failure != nil {
			t.Fatalf("expected success, got failure: %+v", res.Failure)
		}
		if !strings.Contains(res.Output, "found 2 occurrences of auth middleware") {
			t.Errorf("output = %q, want 'found 2 occurrences of auth middleware'", res.Output)
		}
		if !strings.Contains(res.Output, `<subagent_result id=`) {
			t.Errorf("missing subagent_result tag in output: %s", res.Output)
		}
	})

	t.Run("permission detail provider", func(t *testing.T) {
		provider, ok := handler.(tool.DetailProvider)
		if !ok {
			t.Fatal("handler does not implement tool.DetailProvider")
		}
		args, _ := json.Marshal(map[string]any{
			"profile": "explorer",
			"task":    "analyze database queries in repository",
		})
		detail := provider.PermissionDetail(args)
		if detail != "[explorer] analyze database queries in repository" {
			t.Errorf("detail = %q, want '[explorer] analyze database queries in repository'", detail)
		}
	})

	t.Run("invalid profile", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"profile": "invalid_profile",
			"task":    "some task",
		})
		call, _ := tool.NewCall("call-2", "delegate_task", args)

		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for invalid profile")
		}
		if !strings.Contains(err.Error(), "unknown agent profile") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("missing task", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"profile": "reviewer",
			"task":    "",
		})
		call, _ := tool.NewCall("call-3", "delegate_task", args)

		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for empty task")
		}
		if !strings.Contains(err.Error(), "task is required") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		cancelingCoord := agent.NewCoordinator(
			nil,
			nil,
			nil,
			nil,
			agent.WithRunnerFactory(func(p agent.Profile, tools *toolcall.Service) (turn.Runner, error) {
				return &mockDelegateRunner{
					runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
						<-ctx.Done()
						return turn.Result{}, ctx.Err()
					},
				}, nil
			}),
		)
		defer cancelingCoord.Close()

		cHandler := NewDelegateTask(cancelingCoord)
		cancelCtx, cancel := context.WithCancel(context.Background())

		args, _ := json.Marshal(map[string]any{
			"profile": "explorer",
			"task":    "hang task",
		})
		call, _ := tool.NewCall("call-4", "delegate_task", args)

		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		_, err := cHandler.Execute(cancelCtx, call)
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	})

	t.Run("parentID propagation", func(t *testing.T) {
		var receivedParentID string
		parentCoord := agent.NewCoordinator(
			nil,
			nil,
			nil,
			nil,
			agent.WithEventSink(func(ctx context.Context, ev agent.Event) error {
				if ev.Kind == agent.EventAgentStarted {
					receivedParentID = ev.ParentID
				}
				return nil
			}),
			agent.WithRunnerFactory(func(p agent.Profile, tools *toolcall.Service) (turn.Runner, error) {
				return mockSubagentRunner{content: "ok"}, nil
			}),
		)
		defer parentCoord.Close()

		pHandler := NewDelegateTask(parentCoord, "session-xyz")
		args, _ := json.Marshal(map[string]any{
			"profile": "explorer",
			"task":    "test task",
		})
		call, _ := tool.NewCall("call-parent", "delegate_task", args)
		_, err := pHandler.Execute(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		if receivedParentID != "session-xyz" {
			t.Errorf("receivedParentID = %q, want 'session-xyz'", receivedParentID)
		}
	})

	t.Run("pow dex int delegation and schema", func(t *testing.T) {
		def := handler.Definition()
		props, ok := def.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatal("expected properties in InputSchema")
		}
		profileProp, ok := props["profile"].(map[string]any)
		if !ok {
			t.Fatal("expected profile in properties")
		}
		enums, ok := profileProp["enum"].([]string)
		if !ok {
			t.Fatal("expected enum slice in profile property")
		}
		enumMap := make(map[string]bool)
		for _, e := range enums {
			enumMap[e] = true
		}
		for _, expected := range []string{"pow", "dex", "int"} {
			if !enumMap[expected] {
				t.Errorf("delegate_task enum missing %q", expected)
			}
		}

		for _, prof := range []string{"pow", "dex", "int"} {
			args, _ := json.Marshal(map[string]any{
				"profile": prof,
				"task":    "task for " + prof,
			})
			call, _ := tool.NewCall("call-"+prof, "delegate_task", args)
			res, err := handler.Execute(ctx, call)
			if err != nil {
				t.Fatalf("unexpected error for profile %s: %v", prof, err)
			}
			if res.Failure != nil {
				t.Fatalf("unexpected failure for profile %s: %+v", prof, res.Failure)
			}
		}
	})
}

