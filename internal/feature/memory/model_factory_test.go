package memory

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/modelclient"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type captureFactory struct{ model *captureModel }

func (f captureFactory) Build(modelclient.Request) sdk.LanguageModel { return f.model }

type captureModel struct{ requests []sdk.Request }

func (*captureModel) Provider() string { return "test" }
func (*captureModel) ModelID() string  { return "test-model" }
func (*captureModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (m *captureModel) Stream(_ context.Context, request sdk.Request) (sdk.Stream, error) {
	m.requests = append(m.requests, request)
	return &captureStream{}, nil
}

type captureStream struct{ done bool }

func (s *captureStream) Next(context.Context) (sdk.Event, error) {
	if s.done {
		return sdk.Event{}, io.EOF
	}
	s.done = true
	return sdk.Event{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}, nil
}
func (*captureStream) Close() error { return nil }

func TestMemoryModelPrependsContextToCurrentUserWithoutMutatingInput(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "verification tests", Value: "Run go test ./...", Keywords: []string{"test"},
		WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now,
	}}}
	base := &captureModel{}
	factory := NewModelFactory(captureFactory{model: base}, repo, "ws", runtimepolicy.DurableMemory())
	model := factory.Build(modelclient.Request{ModelID: "test-model"})
	messages := []sdk.Message{
		{ID: "system", Role: sdk.RoleSystem, Content: "stable system prompt"},
		{ID: "user-1", Role: sdk.RoleUser, Content: "please run test verification"},
	}
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	if len(base.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(base.requests))
	}
	got := base.requests[0].Messages
	if len(got) != 2 {
		t.Fatalf("messages = %+v, want role sequence preserved", got)
	}
	if got[1].Role != sdk.RoleUser || got[1].ID != "user-1" {
		t.Fatalf("current user identity changed: %+v", got[1])
	}
	if !strings.HasPrefix(got[1].Content, "<proton-memory-context>") || !strings.Contains(got[1].Content, "Run go test ./...") || !strings.HasSuffix(got[1].Content, "please run test verification") {
		t.Fatalf("decorated user message = %q", got[1].Content)
	}
	if len(messages) != 2 || messages[1].Content != "please run test verification" {
		t.Fatalf("input messages mutated: %+v", messages)
	}
}

func TestMemoryModelCachesRetrievalAcrossRoundsForSameUserMessage(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact,
		Key: "test command", Value: "go test ./...", WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now,
	}}}
	base := &captureModel{}
	model := NewModelFactory(captureFactory{model: base}, repo, "ws", runtimepolicy.DurableMemory()).Build(modelclient.Request{ModelID: "test-model"})
	request := sdk.Request{Messages: []sdk.Message{{ID: "user-1", Role: sdk.RoleUser, Content: "test command"}}}
	for range 2 {
		stream, err := model.Stream(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		_ = stream.Close()
	}
	if len(repo.used) != 1 {
		t.Fatalf("usage accounting = %d, want one retrieval for repeated round", len(repo.used))
	}
}
