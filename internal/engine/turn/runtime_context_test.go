package turn

import (
	"context"
	"sync"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type testRuntimeContextProvider struct {
	mu        sync.Mutex
	active    bool
	pending   bool
	ready     []model.Message
	await     chan []model.Message
	finalized int
}

func (p *testRuntimeContextProvider) Drain(context.Context) ([]model.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := model.CloneMessages(p.ready)
	p.ready = nil
	return out, nil
}

func (p *testRuntimeContextProvider) Active(context.Context) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active || p.pending
}

func (p *testRuntimeContextProvider) Pending(context.Context) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pending
}

func (p *testRuntimeContextProvider) Await(ctx context.Context) ([]model.Message, error) {
	select {
	case messages := <-p.await:
		p.mu.Lock()
		p.pending = false
		p.active = false
		p.mu.Unlock()
		return model.CloneMessages(messages), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *testRuntimeContextProvider) Finalize(context.Context) {
	p.mu.Lock()
	p.finalized++
	p.mu.Unlock()
}

func TestLoopInjectsReadyRuntimeContextBeforeRound(t *testing.T) {
	provider := &testRuntimeContextProvider{ready: []model.Message{{
		Role: model.RoleUser, Content: "runtime evidence",
	}}}
	client := &scriptedClient{streams: []scriptedStreamSpec{{events: []sdk.Event{
		{Kind: sdk.EventTextDelta, Text: "integrated"},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
	}}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithRuntimeContextProvider(provider))
	result, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "integrated" || len(client.requests) != 1 {
		t.Fatalf("result=%+v requests=%d", result, len(client.requests))
	}
	found := false
	for _, message := range client.requests[0].Messages {
		if message.Content == "runtime evidence" {
			found = true
			if message.Role != model.RoleUser {
				t.Fatalf("runtime context role=%q", message.Role)
			}
		}
	}
	if !found {
		t.Fatalf("request missing runtime evidence: %#v", client.requests[0].Messages)
	}
	provider.mu.Lock()
	finalized := provider.finalized
	provider.mu.Unlock()
	if finalized != 1 {
		t.Fatalf("runtime context finalized=%d, want 1", finalized)
	}
	for _, message := range result.Messages {
		if message.Content == "runtime evidence" {
			t.Fatal("ephemeral runtime context leaked into persisted turn messages")
		}
	}
}

func TestLoopDefersFinalTextUntilPendingRuntimeContextArrives(t *testing.T) {
	provider := &testRuntimeContextProvider{pending: true, await: make(chan []model.Message, 1)}
	provider.await <- []model.Message{{Role: model.RoleUser, Content: "child result"}}
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "premature"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "integrated result"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithRuntimeContextProvider(provider))
	events := make([]Event, 0)
	result, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "delegate"}}, collectEvents(&events))
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "integrated result" || result.Rounds != 2 {
		t.Fatalf("result=%+v", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests=%d", len(client.requests))
	}
	found := false
	for _, message := range client.requests[1].Messages {
		if message.Content == "child result" {
			found = true
		}
	}
	if !found {
		t.Fatalf("second request missing child result: %#v", client.requests[1].Messages)
	}
	for _, event := range events {
		if event.Kind == EventTextDelta && event.Text == "premature" {
			t.Fatal("premature final text escaped completion barrier")
		}
	}
}

type stagedOptionalRuntimeContext struct {
	mu        sync.Mutex
	drains    int
	active    bool
	finalized int
}

func (p *stagedOptionalRuntimeContext) Drain(context.Context) ([]model.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.drains++
	if p.drains == 2 {
		p.active = false
		return []model.Message{{Role: model.RoleUser, Content: "optional child result"}}, nil
	}
	return nil, nil
}
func (p *stagedOptionalRuntimeContext) Await(context.Context) ([]model.Message, error) {
	return nil, nil
}
func (p *stagedOptionalRuntimeContext) Active(context.Context) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active
}
func (*stagedOptionalRuntimeContext) Pending(context.Context) bool { return false }
func (p *stagedOptionalRuntimeContext) Finalize(context.Context) {
	p.mu.Lock()
	p.finalized++
	p.mu.Unlock()
}

func TestLoopIntegratesOptionalResultThatBecomesReadyDuringBufferedRound(t *testing.T) {
	provider := &stagedOptionalRuntimeContext{active: true}
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "premature"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "integrated optional"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithRuntimeContextProvider(provider))
	events := make([]Event, 0)
	result, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "delegate optional"}}, collectEvents(&events))
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "integrated optional" || result.Rounds != 2 {
		t.Fatalf("result=%+v", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests=%d, want 2", len(client.requests))
	}
	found := false
	for _, message := range client.requests[1].Messages {
		found = found || message.Content == "optional child result"
	}
	if !found {
		t.Fatalf("second request missing optional result: %#v", client.requests[1].Messages)
	}
	for _, event := range events {
		if event.Kind == EventTextDelta && event.Text == "premature" {
			t.Fatal("buffered tentative text escaped before optional result integration")
		}
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.finalized != 1 {
		t.Fatalf("finalized=%d, want 1", provider.finalized)
	}
}

func TestLoopFinalizesRuntimeContextOnModelFailure(t *testing.T) {
	provider := &testRuntimeContextProvider{}
	client := &scriptedClient{}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithRuntimeContextProvider(provider))
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "fail"}}, nil); err == nil {
		t.Fatal("expected model failure")
	}
	provider.mu.Lock()
	finalized := provider.finalized
	provider.mu.Unlock()
	if finalized != 1 {
		t.Fatalf("runtime context finalized=%d, want 1", finalized)
	}
}
