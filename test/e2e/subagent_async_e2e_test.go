package e2e_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/tool/builtin"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
)

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
	registry, err := builtin.NewRegistry(
		builtin.NewDelegateTask(coord),
		builtin.NewWaitAgent(coord),
		builtin.NewGetAgent(coord),
		builtin.NewListAgents(coord),
		builtin.NewCancelAgent(coord),
	)
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
	encoded, _ := json.Marshal(args)
	call, err := tool.NewCall(id, name, encoded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Call(context.Background(), call)
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

	spawn := callAgentTool(t, service, "spawn", "delegate_task", map[string]any{"profile": "explorer", "task": "inspect asynchronously"})
	var handle struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(spawn.Output), &handle); err != nil || handle.AgentID == "" {
		t.Fatalf("spawn=%s err=%v", spawn.Output, err)
	}

	wait := callAgentTool(t, service, "wait-1", "wait_agent", map[string]any{"agent_id": handle.AgentID})
	if !strings.Contains(wait.Output, `"status":"running"`) && !strings.Contains(wait.Output, `"status":"queued"`) {
		t.Fatalf("first wait=%s", wait.Output)
	}
	if _, ok := coord.Get(handle.AgentID); !ok {
		t.Fatal("wait timeout removed child")
	}

	close(release)
	wait = callAgentTool(t, service, "wait-2", "wait_agent", map[string]any{"agent_id": handle.AgentID, "timeout_seconds": 1})
	if !strings.Contains(wait.Output, `"status":"completed"`) || !strings.Contains(wait.Output, "persistent result") {
		t.Fatalf("completed wait=%s", wait.Output)
	}
	get := callAgentTool(t, service, "get", "get_agent", map[string]any{"agent_id": handle.AgentID})
	if !strings.Contains(get.Output, `"state":"completed"`) || !strings.Contains(get.Output, "persistent result") {
		t.Fatalf("get=%s", get.Output)
	}
	list := callAgentTool(t, service, "list", "list_agents", map[string]any{})
	if !strings.Contains(list.Output, handle.AgentID) || !strings.Contains(list.Output, `"state":"completed"`) {
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
	spawn := callAgentTool(t, service, "spawn", "delegate_task", map[string]any{"profile": "explorer", "task": "cancel me"})
	var handle struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.Unmarshal([]byte(spawn.Output), &handle)
	callAgentTool(t, service, "cancel", "cancel_agent", map[string]any{"agent_id": handle.AgentID})
	wait := callAgentTool(t, service, "wait", "wait_agent", map[string]any{"agent_id": handle.AgentID, "timeout_seconds": 1})
	if !strings.Contains(wait.Output, `"status":"canceled"`) {
		t.Fatalf("wait after cancel=%s", wait.Output)
	}
}
