package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

type semanticResultRunner struct{}

func (semanticResultRunner) Run(ctx context.Context, _ []model.Message, sink turn.Sink) (turn.Result, error) {
	call, err := tool.NewCall("read-1", "read", json.RawMessage(`{"path":"session.go"}`))
	if err != nil {
		return turn.Result{}, err
	}
	if err := sink(ctx, turn.Event{
		Kind:   turn.EventToolResult,
		Call:   call,
		Result: tool.Result{CallID: call.ID, ToolName: call.Name, Output: "observed"},
	}); err != nil {
		return turn.Result{}, err
	}
	content := `<proton-subagent-result>{"conclusion":"reconnect is duplicated","findings":[{"claim":"listener is registered twice","confidence":"high","evidence":[{"tool":"read","target":"session.go"},{"tool":"read","target":"fake.go"}]}]}</proton-subagent-result>`
	return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: content}, Rounds: 1}, nil
}
func TestExecuteBuildsStructuredResultFromValidatedEvidence(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return semanticResultRunner{}, nil
		}),
	)
	defer coord.Close()

	result, err := coord.execute(context.Background(), Request{ID: "agility-1", Profile: ProfileAgility, Task: "inspect reconnect"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Conclusion != "reconnect is duplicated" || result.Summary != result.Conclusion {
		t.Fatalf("result conclusion=%q summary=%q", result.Conclusion, result.Summary)
	}
	if len(result.Findings) != 1 || result.Findings[0].Claim != "listener is registered twice" {
		t.Fatalf("findings=%#v", result.Findings)
	}
	if len(result.Findings[0].Evidence) != 1 || result.Findings[0].Evidence[0].Target != "session.go" {
		t.Fatalf("validated finding evidence=%#v", result.Findings[0].Evidence)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Target != "session.go" {
		t.Fatalf("runtime evidence=%#v", result.Evidence)
	}
}
