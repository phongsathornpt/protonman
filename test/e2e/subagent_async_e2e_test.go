package e2e_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	agenttool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/agent"
	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type functionLifecycleRunner struct {
	run func(context.Context, []model.Message, turn.Sink) (turn.Result, error)
}

func (r functionLifecycleRunner) Run(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
	return r.run(ctx, messages, sink)
}

type asyncLifecycleRunner struct {
	release <-chan struct{}
	content string
}

func (r asyncLifecycleRunner) Run(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
	select {
	case <-r.release:
		return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: r.content}, Rounds: 2}, nil
	case <-ctx.Done():
		return turn.Result{}, ctx.Err()
	}
}

func agentLifecycleService(t *testing.T, coord *agent.Coordinator) *toolcall.Service {
	t.Helper()
	registry, err := builtin.NewRegistry(agenttool.NewSubagent(coord))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(permission.ModeAsk))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func callAgentTool(t *testing.T, service *toolcall.Service, id, name string, args map[string]any) tool.Result {
	t.Helper()
	return callAgentToolContext(t, context.Background(), service, id, name, args)
}

func callAgentToolContext(t *testing.T, ctx context.Context, service *toolcall.Service, id, name string, args map[string]any) tool.Result {
	t.Helper()
	encoded, _ := json.Marshal(args)
	call, err := tool.NewCall(id, name, encoded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Call(ctx, call)
	if err != nil {
		t.Fatalf("%s error: %v", name, err)
	}
	return result
}

func TestE2EAsyncSubagentWaitDoesNotCancel(t *testing.T) {
	release := make(chan struct{})
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithDefaultWaitTimeout(20*time.Millisecond),
		agent.WithMaxRuntime(time.Second),
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return asyncLifecycleRunner{release: release, content: "persistent result"}, nil
		}),
	)
	defer coord.Close()
	service := agentLifecycleService(t, coord)

	spawn := callAgentTool(t, service, "spawn", "subagent", map[string]any{"action": "spawn", "profile": "agility", "task": "inspect asynchronously"})
	var handle struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(spawn.StructuredOutput, &handle); err != nil || handle.AgentID == "" {
		t.Fatalf("spawn=%s err=%v", spawn.Output, err)
	}

	wait := callAgentTool(t, service, "wait-1", "subagent", map[string]any{"action": "wait"})
	if !strings.Contains(string(wait.StructuredOutput), `"timed_out":true`) {
		t.Fatalf("first wait=%s structured=%s", wait.Output, wait.StructuredOutput)
	}
	if _, ok := coord.Get(handle.AgentID); !ok {
		t.Fatal("wait timeout removed child")
	}

	close(release)
	wait = callAgentTool(t, service, "wait-2", "subagent", map[string]any{"action": "wait", "timeout_seconds": 10})
	if !strings.Contains(string(wait.StructuredOutput), `"timed_out":false`) || !strings.Contains(string(wait.StructuredOutput), "persistent result") || !strings.Contains(string(wait.StructuredOutput), handle.AgentID) {
		t.Fatalf("completed wait=%s structured=%s", wait.Output, wait.StructuredOutput)
	}
	get := callAgentTool(t, service, "get", "subagent", map[string]any{"action": "get", "agent_id": handle.AgentID})
	if !strings.Contains(string(get.StructuredOutput), `"state":"completed"`) || !strings.Contains(string(get.StructuredOutput), "persistent result") {
		t.Fatalf("get=%s", get.Output)
	}
	list := callAgentTool(t, service, "list", "subagent", map[string]any{"action": "list"})
	if !strings.Contains(string(list.StructuredOutput), handle.AgentID) || !strings.Contains(string(list.StructuredOutput), `"state":"completed"`) {
		t.Fatalf("list=%s", list.Output)
	}
}

func TestE2EAsyncSubagentExplicitCancel(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return asyncLifecycleRunner{release: make(chan struct{}), content: "never"}, nil
		}),
	)
	defer coord.Close()
	service := agentLifecycleService(t, coord)
	spawn := callAgentTool(t, service, "spawn", "subagent", map[string]any{"action": "spawn", "profile": "agility", "task": "cancel me"})
	var handle struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.Unmarshal(spawn.StructuredOutput, &handle)
	callAgentTool(t, service, "cancel", "subagent", map[string]any{"action": "cancel", "agent_id": handle.AgentID})
	wait := callAgentTool(t, service, "wait", "subagent", map[string]any{"action": "wait", "timeout_seconds": 10})
	if !strings.Contains(string(wait.StructuredOutput), `"agent_failed"`) || !strings.Contains(string(wait.StructuredOutput), handle.AgentID) {
		t.Fatalf("wait after cancel=%s structured=%s", wait.Output, wait.StructuredOutput)
	}
}

func TestE2EAgentWaitIsScopedToParentTurn(t *testing.T) {
	releaseA := make(chan struct{})
	releaseB := make(chan struct{})
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithDefaultWaitTimeout(20*time.Millisecond),
		agent.WithMaxRuntime(time.Second),
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return &functionLifecycleRunner{run: func(ctx context.Context, messages []model.Message, _ turn.Sink) (turn.Result, error) {
				release := releaseB
				content := "parent B done"
				for _, message := range messages {
					if strings.Contains(message.Content, "parent A") {
						release = releaseA
						content = "parent A done"
						break
					}
				}
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: content}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer coord.Close()
	service := agentLifecycleService(t, coord)
	ctxA := agent.WithTurnRef(context.Background(), agent.TurnRef{TurnID: "turn-a"})
	ctxB := agent.WithTurnRef(context.Background(), agent.TurnRef{TurnID: "turn-b"})

	spawnA := callAgentToolContext(t, ctxA, service, "spawn-a", "subagent", map[string]any{"action": "spawn", "profile": "agility", "task": "parent A"})
	spawnB := callAgentToolContext(t, ctxB, service, "spawn-b", "subagent", map[string]any{"action": "spawn", "profile": "agility", "task": "parent B"})
	if !strings.Contains(string(spawnA.StructuredOutput), `"agent_id"`) || !strings.Contains(string(spawnB.StructuredOutput), `"agent_id"`) {
		t.Fatalf("spawn outputs missing agent ids: A=%s B=%s", spawnA.StructuredOutput, spawnB.StructuredOutput)
	}

	close(releaseB)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		found := false
		for _, status := range coord.List() {
			if status.ParentID == "turn-b" && status.State.Terminal() {
				found = true
				break
			}
		}
		if found {
			break
		}
		time.Sleep(time.Millisecond)
	}

	waitA := callAgentToolContext(t, ctxA, service, "wait-a-1", "subagent", map[string]any{"action": "wait"})
	if !strings.Contains(string(waitA.StructuredOutput), `"timed_out":true`) || strings.Contains(string(waitA.StructuredOutput), "parent B done") {
		t.Fatalf("unrelated child woke parent A: %s", waitA.StructuredOutput)
	}

	close(releaseA)
	waitA = callAgentToolContext(t, ctxA, service, "wait-a-2", "subagent", map[string]any{"action": "wait"})
	if !strings.Contains(string(waitA.StructuredOutput), `"timed_out":false`) || !strings.Contains(string(waitA.StructuredOutput), "parent A done") || strings.Contains(string(waitA.StructuredOutput), "parent B done") {
		t.Fatalf("parent A completion wait=%s", waitA.StructuredOutput)
	}
}
