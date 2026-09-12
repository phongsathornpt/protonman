package agenttool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
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

func TestDelegateTaskExecute(t *testing.T) {
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
			"profile": "agility",
			"task":    "search for auth middleware",
			"task_id": "inspect-auth",
		})
		call, err := tool.NewCall("call-1", "subagent", args)
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
		var spawned struct {
			AgentID string      `json:"agent_id"`
			TaskID  string      `json:"task_id"`
			Status  agent.State `json:"status"`
		}
		if err := json.Unmarshal(res.StructuredOutput, &spawned); err != nil {
			t.Fatalf("decode spawn output: %v", err)
		}
		if spawned.AgentID == "" || spawned.TaskID != "inspect-auth" || (spawned.Status != agent.StateQueued && spawned.Status != agent.StateRunning) {
			t.Fatalf("spawn output = %s", res.StructuredOutput)
		}
		status, ok := coord.Get(spawned.AgentID)
		if !ok || status.TaskID != "inspect-auth" {
			t.Fatalf("agent status task linkage = %+v found=%v", status, ok)
		}
		wr, err := coord.Wait(context.Background(), spawned.AgentID, time.Second)
		if err != nil || wr.Result == nil || !strings.Contains(wr.Result.Summary, "found 2 occurrences of auth middleware") {
			t.Fatalf("wait result = %+v err=%v", wr, err)
		}
	})

	t.Run("optional delegation propagates barrier policy", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"profile":  "agility",
			"task":     "speculative lookup",
			"optional": true,
		})
		call, _ := tool.NewCall("call-optional", "subagent", args)
		res, err := handler.Execute(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		var spawned struct {
			AgentID  string `json:"agent_id"`
			Optional bool   `json:"optional"`
		}
		if err := json.Unmarshal(res.StructuredOutput, &spawned); err != nil {
			t.Fatal(err)
		}
		status, ok := coord.Get(spawned.AgentID)
		if !ok || !spawned.Optional || !status.Optional {
			t.Fatalf("spawned=%+v status=%+v", spawned, status)
		}
	})

	t.Run("permission detail provider", func(t *testing.T) {
		provider, ok := handler.(tool.DetailProvider)
		if !ok {
			t.Fatal("handler does not implement tool.DetailProvider")
		}
		args, _ := json.Marshal(map[string]any{
			"profile": "agility",
			"task":    "analyze database queries in repository",
		})
		detail := provider.PermissionDetail(args)
		if detail != "[agility] analyze database queries in repository" {
			t.Errorf("detail = %q, want '[agility] analyze database queries in repository'", detail)
		}
	})

	t.Run("invalid profile", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"profile": "invalid_profile",
			"task":    "some task",
		})
		call, _ := tool.NewCall("call-2", "subagent", args)

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
			"profile": "agility",
			"task":    "",
		})
		call, _ := tool.NewCall("call-3", "subagent", args)

		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for empty task")
		}
		if !strings.Contains(err.Error(), "task is required") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		cHandler := NewDelegateTask(coord)
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()
		args, _ := json.Marshal(map[string]any{"profile": "agility", "task": "hang task"})
		call, _ := tool.NewCall("call-4", "subagent", args)
		if _, err := cHandler.Execute(cancelCtx, call); err == nil {
			t.Fatal("expected canceled submission error")
		}
	})

	t.Run("parentID propagation", func(t *testing.T) {
		parentCoord := agent.NewCoordinator(
			nil,
			nil,
			nil,
			nil,
			agent.WithRunnerFactory(func(p agent.Profile, tools *toolcall.Service) (turn.Runner, error) {
				return mockSubagentRunner{content: "ok"}, nil
			}),
		)
		defer parentCoord.Close()

		pHandler := NewDelegateTask(parentCoord, "session-xyz")
		args, _ := json.Marshal(map[string]any{
			"profile": "agility",
			"task":    "test task",
		})
		call, _ := tool.NewCall("call-parent", "subagent", args)
		res, err := pHandler.Execute(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		var spawned struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.Unmarshal(res.StructuredOutput, &spawned); err != nil {
			t.Fatal(err)
		}
		status, ok := parentCoord.Get(spawned.AgentID)
		if !ok || status.ParentID != "session-xyz" {
			t.Fatalf("status = %+v, want parent session-xyz", status)
		}
		_, _ = parentCoord.Wait(context.Background(), spawned.AgentID, time.Second)
	})

	t.Run("dota attribute delegation and schema", func(t *testing.T) {
		def := handler.Definition()
		props, ok := def.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatal("expected properties in InputSchema")
		}
		profileProp, ok := props["profile"].(map[string]any)
		if !ok {
			t.Fatal("expected profile in properties")
		}
		if optional, ok := props["optional"].(map[string]any); !ok || optional["type"] != "boolean" {
			t.Fatalf("optional schema = %#v, want boolean", props["optional"])
		}
		dependsOn, ok := props["depends_on"].(map[string]any)
		if !ok || dependsOn["type"] != "array" || dependsOn["maxItems"] != agent.MaxAgentDependencies {
			t.Fatalf("depends_on schema = %#v", props["depends_on"])
		}
		enums, ok := profileProp["enum"].([]string)
		if !ok {
			t.Fatal("expected enum slice in profile property")
		}
		enumMap := make(map[string]bool)
		for _, e := range enums {
			enumMap[e] = true
		}
		for _, expected := range []string{"strength", "intelligence", "agility"} {
			if !enumMap[expected] {
				t.Errorf("subagent enum missing %q", expected)
			}
		}

		for _, prof := range []string{"strength", "intelligence", "agility"} {
			args, _ := json.Marshal(map[string]any{
				"profile": prof,
				"task":    "task for " + prof,
			})
			call, _ := tool.NewCall("call-"+prof, "subagent", args)
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

func TestDelegateTaskUsesCallerBoundedExecutionTimeout(t *testing.T) {
	definition := NewDelegateTask(nil).Definition()
	if definition.ExecutionTimeoutPolicy != tool.ExecutionTimeoutCallerBounded {
		t.Fatalf("execution timeout policy = %q, want caller bounded", definition.ExecutionTimeoutPolicy)
	}
}

func TestDelegateTaskAppliesRequestedShorterTimeout(t *testing.T) {
	deadlineCh := make(chan time.Duration, 1)
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithMaxRuntime(5*time.Second),
		agent.WithEventSink(func(ctx context.Context, ev agent.Event) error {
			if ev.Kind == agent.EventAgentStarted {
				if deadline, ok := ctx.Deadline(); ok {
					deadlineCh <- time.Until(deadline)
				}
			}
			return nil
		}),
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return mockSubagentRunner{content: "ok"}, nil
		}),
	)
	defer coord.Close()
	handler := NewDelegateTask(coord)
	args, _ := json.Marshal(map[string]any{"profile": "agility", "task": "short", "timeout_seconds": 1})
	call, _ := tool.NewCall("short-timeout", "subagent", args)
	if _, err := handler.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	remaining := <-deadlineCh
	if remaining <= 0 || remaining > 1500*time.Millisecond {
		t.Fatalf("execution deadline remaining = %v, want about 1s", remaining)
	}
}

func TestDelegateTaskRejectsExcessiveTimeoutSeconds(t *testing.T) {
	handler := NewDelegateTask(agent.NewCoordinator(nil, nil, nil, nil))
	args, _ := json.Marshal(map[string]any{"profile": "agility", "task": "too long", "timeout_seconds": 86401})
	call, _ := tool.NewCall("bad-timeout", "subagent", args)
	if _, err := handler.Execute(context.Background(), call); err == nil || !strings.Contains(err.Error(), "timeout_seconds") {
		t.Fatalf("Execute() error = %v, want timeout validation error", err)
	}
}

func TestDelegateTaskPrefersContextParentID(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return mockSubagentRunner{content: "ok"}, nil
		}),
	)
	defer coord.Close()

	handler := NewDelegateTask(coord, "fallback-parent")
	ctx := agent.WithTurnRef(context.Background(), agent.TurnRef{TurnID: "turn-7"})
	args, _ := json.Marshal(map[string]any{"profile": "agility", "task": "inspect"})
	call, _ := tool.NewCall("call-context-parent", "subagent", args)
	res, err := handler.Execute(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	var spawned struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(res.StructuredOutput, &spawned); err != nil {
		t.Fatal(err)
	}
	status, ok := coord.Get(spawned.AgentID)
	if !ok || status.ParentID != "turn-7" {
		t.Fatalf("status=%+v, want parent turn-7", status)
	}
}

func TestSubagentDefinitionDescribesEventDrivenResultDelivery(t *testing.T) {
	def := NewSubagent(nil).Definition()
	for _, want := range []string{"completed results are delivered automatically", "wait/get/list are diagnostic", "cancel/resume"} {
		if !strings.Contains(def.Description, want) {
			t.Fatalf("subagent description missing %q: %q", want, def.Description)
		}
	}
	props, ok := def.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("subagent input schema properties missing")
	}
	action, ok := props["action"].(map[string]any)
	if !ok {
		t.Fatal("subagent action schema missing")
	}
	description, _ := action["description"].(string)
	for _, want := range []string{"automatic result delivery", "diagnostic inspection"} {
		if !strings.Contains(description, want) {
			t.Fatalf("subagent action description missing %q: %q", want, description)
		}
	}
}
