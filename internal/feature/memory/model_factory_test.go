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
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	port "github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
)

type captureFactory struct{ model *captureModel }

func (f captureFactory) Build(modelclient.Request) port.LanguageModel { return f.model }

type captureModel struct{ requests []domain.Request }

func (*captureModel) Provider() string { return "test" }
func (*captureModel) ModelID() string  { return "test-model" }
func (*captureModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}
func (m *captureModel) Stream(_ context.Context, request domain.Request) (port.Stream, error) {
	m.requests = append(m.requests, request)
	return &captureStream{}, nil
}

type captureStream struct{ done bool }

func (s *captureStream) Next(context.Context) (domain.Event, error) {
	if s.done {
		return domain.Event{}, io.EOF
	}
	s.done = true
	return domain.Event{Kind: domain.EventFinish, FinishReason: domain.FinishStop}, nil
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
	var activities []corememory.Activity
	ctx := corememory.WithActivitySink(context.Background(), func(_ context.Context, activity corememory.Activity) error {
		activities = append(activities, activity)
		return nil
	})
	messages := []domain.Message{
		{ID: "system", Role: domain.RoleSystem, Content: "stable system prompt"},
		{ID: "user-1", Role: domain.RoleUser, Content: "please run test verification"},
	}
	stream, err := model.Stream(ctx, domain.Request{Messages: messages})
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
	if got[1].Role != domain.RoleUser || got[1].ID != "user-1" {
		t.Fatalf("current user identity changed: %+v", got[1])
	}
	if !strings.HasPrefix(got[1].Content, "<proton-memory-context>") || !strings.Contains(got[1].Content, "Run go test ./...") || !strings.HasSuffix(got[1].Content, "please run test verification") {
		t.Fatalf("decorated user message = %q", got[1].Content)
	}
	if len(messages) != 2 || messages[1].Content != "please run test verification" {
		t.Fatalf("input messages mutated: %+v", messages)
	}
	if len(activities) != 1 || activities[0] != (corememory.Activity{Kind: corememory.ActivityContextIncluded, WorkspaceEntries: 1}) {
		t.Fatalf("memory activities = %+v, want one included workspace memory", activities)
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
	request := domain.Request{Messages: []domain.Message{{ID: "user-1", Role: domain.RoleUser, Content: "test command"}}}
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
