package runtime

import (
	"context"
	"io"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type goalTestModel struct{}

func (*goalTestModel) Provider() string { return "test" }
func (*goalTestModel) ModelID() string  { return "goal-test" }
func (*goalTestModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true}
}
func (*goalTestModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	return &goalTestStream{}, nil
}

type goalTestStream struct{ index int }

func (s *goalTestStream) Next(context.Context) (sdk.Event, error) {
	switch s.index {
	case 0:
		s.index++
		return sdk.Event{Kind: sdk.EventTextDelta, Text: "done"}, nil
	case 1:
		s.index++
		return sdk.Event{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}, nil
	default:
		return sdk.Event{}, io.EOF
	}
}
func (*goalTestStream) Close() error { return nil }

func TestGoalCommandStartsExecutionTurn(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	runner, err := turn.NewLoop(&goalTestModel{}, service, turn.WithSystemPromptSpec(prompt.Spec{}))
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(context.Background(), service, registry, nil, runner, newPermissionBridge(), "")
	cmd := m.executeCommand("/goal implement retry recovery")
	if cmd == nil {
		t.Fatal("/goal with detail did not start a turn")
	}
	if !m.busy {
		t.Fatal("/goal with detail did not mark the turn busy")
	}
	if len(m.messages) != 1 || m.messages[0].Content != "implement retry recovery" {
		t.Fatalf("goal execution messages = %#v", m.messages)
	}
}
