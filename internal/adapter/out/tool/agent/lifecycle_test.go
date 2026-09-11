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

func TestWaitAgentTimeoutDoesNotCancelChild(t *testing.T) {
	release := make(chan struct{})
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithDefaultWaitTimeout(20*time.Millisecond),
		agent.WithMaxRuntime(time.Second),
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockDelegateRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "finished"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer coord.Close()

	h, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileAgility, Task: "background"})
	if err != nil {
		t.Fatal(err)
	}
	wait := NewWaitAgent(coord)
	call, _ := tool.NewCall("wait-1", "subagent", json.RawMessage(`{}`))
	res, err := wait.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.StructuredOutput), `"timed_out":true`) {
		t.Fatalf("wait output = %s structured=%s", res.Output, res.StructuredOutput)
	}
	if _, ok := coord.Get(h.ID); !ok {
		t.Fatal("wait timeout canceled or removed child")
	}

	close(release)
	call2, _ := tool.NewCall("wait-2", "subagent", json.RawMessage(`{"timeout_seconds":10}`))
	res, err = wait.Execute(context.Background(), call2)
	if err != nil {
		t.Fatal(err)
	}
	structured := string(res.StructuredOutput)
	if !strings.Contains(structured, `"timed_out":false`) || !strings.Contains(structured, h.ID) {
		t.Fatalf("completion output = %s structured=%s", res.Output, res.StructuredOutput)
	}
	if strings.Contains(structured, "finished") {
		t.Fatalf("wait leaked retained child result: %s", structured)
	}
}

func TestAgentLifecycleGetListCancel(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockDelegateRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				<-ctx.Done()
				return turn.Result{}, ctx.Err()
			}}, nil
		}),
	)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileAgility, Task: "cancel target"})
	if err != nil {
		t.Fatal(err)
	}

	getCall, _ := tool.NewCall("get", "subagent", json.RawMessage(`{"agent_id":"`+h.ID+`"}`))
	if res, err := NewGetAgent(coord).Execute(context.Background(), getCall); err != nil || !strings.Contains(string(res.StructuredOutput), h.ID) {
		t.Fatalf("get output=%s err=%v", res.Output, err)
	}
	listCall, _ := tool.NewCall("list", "subagent", json.RawMessage(`{}`))
	if res, err := NewListAgents(coord).Execute(context.Background(), listCall); err != nil || !strings.Contains(string(res.StructuredOutput), h.ID) {
		t.Fatalf("list output=%s err=%v", res.Output, err)
	}
	cancelCall, _ := tool.NewCall("cancel", "subagent", json.RawMessage(`{"agent_id":"`+h.ID+`"}`))
	if _, err := NewCancelAgent(coord).Execute(context.Background(), cancelCall); err != nil {
		t.Fatal(err)
	}
	wr, err := coord.Wait(context.Background(), h.ID, time.Second)
	if err != nil || wr.State != agent.StateCanceled {
		t.Fatalf("wait=%+v err=%v", wr, err)
	}
	res, err := NewGetAgent(coord).Execute(context.Background(), getCall)
	if err != nil || !strings.Contains(string(res.StructuredOutput), `"state":"canceled"`) {
		t.Fatalf("terminal get=%s err=%v", res.Output, err)
	}
}
