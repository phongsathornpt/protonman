package turn

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type stagedProgressRuntimeContext struct {
	drains int
}

func (p *stagedProgressRuntimeContext) Drain(context.Context) ([]model.Message, error) {
	p.drains++
	if p.drains == 3 {
		return []model.Message{{Role: model.RoleUser, Content: "delegated workspace mutation completed"}}, nil
	}
	return nil, nil
}

func (*stagedProgressRuntimeContext) Await(context.Context) ([]model.Message, error) {
	return nil, nil
}

func (*stagedProgressRuntimeContext) Active(context.Context) bool { return false }
func (*stagedProgressRuntimeContext) Pending(context.Context) bool { return false }
func (*stagedProgressRuntimeContext) Finalize(context.Context)     {}

func TestLoopRuntimeContextInvalidatesSuppressedReadObservation(t *testing.T) {
	provider := &stagedProgressRuntimeContext{}
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: repeatedReadEvents("read-1")},
		{events: repeatedReadEvents("read-2")},
		{events: repeatedReadEvents("read-3")},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "verified after delegated change"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow, WithRuntimeContextProvider(provider))

	result, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect after delegated change"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(handler.calls), 3; got != want {
		t.Fatalf("physical read calls = %d, want %d after runtime context invalidation", got, want)
	}
	if got, want := result.Rounds, 4; got != want {
		t.Fatalf("rounds = %d, want %d", got, want)
	}
	if result.Message.Content != "verified after delegated change" {
		t.Fatalf("final response = %q", result.Message.Content)
	}
	if got, want := len(client.requests), 4; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	foundRuntimeContext := false
	for _, message := range client.requests[2].Messages {
		if message.Content == "delegated workspace mutation completed" {
			foundRuntimeContext = true
			break
		}
	}
	if !foundRuntimeContext {
		t.Fatalf("third request missing runtime context: %#v", client.requests[2].Messages)
	}
}
