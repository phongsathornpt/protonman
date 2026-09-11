package e2e_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestE2ESubagentDependsOnGatesExecution(t *testing.T) {
	upstreamRelease := make(chan struct{})
	downstreamStarted := make(chan struct{})
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return functionLifecycleRunner{run: func(ctx context.Context, messages []model.Message, _ turn.Sink) (turn.Result, error) {
				prompt := messages[len(messages)-1].Content
				if strings.Contains(prompt, "Task: upstream") {
					select {
					case <-upstreamRelease:
					case <-ctx.Done():
						return turn.Result{}, ctx.Err()
					}
				}
				if strings.Contains(prompt, "Task: downstream") {
					close(downstreamStarted)
				}
				return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
			}}, nil
		}),
	)
	defer coord.Close()
	service := agentLifecycleService(t, coord)
	ctx := agent.WithTurnRef(context.Background(), agent.TurnRef{SessionID: "session-dag", TurnID: "turn-dag"})

	upstreamSpawn := callAgentToolContext(t, ctx, service, "spawn-upstream", "subagent", map[string]any{
		"action": "spawn", "profile": "agility", "task": "upstream",
	})
	var upstream struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(upstreamSpawn.StructuredOutput, &upstream); err != nil || upstream.AgentID == "" {
		t.Fatalf("upstream spawn=%s err=%v", upstreamSpawn.StructuredOutput, err)
	}

	downstreamSpawn := callAgentToolContext(t, ctx, service, "spawn-downstream", "subagent", map[string]any{
		"action": "spawn", "profile": "agility", "task": "downstream", "depends_on": []string{upstream.AgentID},
	})
	var downstream struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(downstreamSpawn.StructuredOutput, &downstream); err != nil || downstream.AgentID == "" {
		t.Fatalf("downstream spawn=%s err=%v", downstreamSpawn.StructuredOutput, err)
	}

	select {
	case <-downstreamStarted:
		t.Fatal("downstream started before dependency completed")
	case <-time.After(30 * time.Millisecond):
	}
	close(upstreamRelease)
	wr, err := coord.Wait(context.Background(), downstream.AgentID, time.Second)
	if err != nil || wr.Result == nil || wr.Result.Err != nil {
		t.Fatalf("downstream wait=%+v err=%v", wr, err)
	}
}
