package app

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelclient"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type emptyToolRegistry struct{}

func (emptyToolRegistry) Lookup(string) (tool.Handler, bool) { return nil, false }
func (emptyToolRegistry) Definitions() []tool.Definition     { return nil }

func testToolService(t *testing.T) *toolcall.Service {
	t.Helper()
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	service, err := toolcall.NewService(emptyToolRegistry{}, policy, toolcall.WithMode(permission.ModeAlwaysApprove))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type recordingModel struct {
	id       string
	requests []sdk.Request
}

func (m *recordingModel) Provider() string { return "test" }
func (m *recordingModel) ModelID() string  { return m.id }
func (m *recordingModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (m *recordingModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	m.requests = append(m.requests, request)
	return closedStream{}, nil
}

type closedStream struct{}

func (closedStream) Next(context.Context) (sdk.Event, error) { return sdk.Event{}, context.Canceled }
func (closedStream) Close() error                            { return nil }

// decoratingFactory mimics a root-only model decorator (such as durable memory)
// that exposes its undecorated base factory for subagent admission.
type decoratingFactory struct {
	base  modelclient.Factory
	built []*recordingModel
}

func (f *decoratingFactory) BaseFactory() LanguageModelFactory { return f.base }

func (f *decoratingFactory) Build(request LanguageModelRequest) sdk.LanguageModel {
	// The decorator builds the base once and returns a decorated wrapper; the
	// base instance is what subagents must receive instead.
	_ = f.base.Build(request)
	clone := &recordingModel{id: "decorated"}
	f.built = append(f.built, clone)
	return clone
}

type staticFactory struct{ model *recordingModel }

func (f staticFactory) Build(LanguageModelRequest) sdk.LanguageModel { return f.model }

// TestBuildConversationGivesSubagentsTheUndecoratedBaseModel locks the invariant
// that a root-only model decorator does not leak into subagent admission. The
// coordinator, which children inherit from, must receive the base model.
func TestBuildConversationGivesSubagentsTheUndecoratedBaseModel(t *testing.T) {
	base := &recordingModel{id: "base"}
	factory := &decoratingFactory{base: staticFactory{model: base}}
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()

	service := testToolService(t)
	if _, err := BuildConversation(service, nil, NewAgents(coord), ConversationSpec{
		ProviderName: "test", ModelID: "m", ModelFactory: factory,
	}); err != nil {
		t.Fatal(err)
	}
	got := coord.LanguageModel()
	if got == nil {
		t.Fatal("coordinator language model was not set")
	}
	if recording, ok := got.(*recordingModel); ok {
		if recording.id != "base" {
			t.Fatalf("coordinator inherited %q, want undecorated base", recording.id)
		}
		// The model handed to children must not carry memory decoration.
		stream, err := got.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{
			{ID: "u", Role: sdk.RoleUser, Content: "please run test verification"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		_ = stream.Close()
		if len(base.requests) != 1 || strings.Contains(base.requests[0].Messages[0].Content, "<proton-memory-context>") {
			t.Fatalf("child-inherited model decorated a child turn: %+v", base.requests)
		}
		return
	}
	t.Fatalf("coordinator language model = %T, want *recordingModel", got)
}

// TestBuildConversationKeepsDecoratorForPlainFactory verifies that factories
// without the base capability are passed through unchanged.
func TestBuildConversationKeepsDecoratorForPlainFactory(t *testing.T) {
	model := &recordingModel{id: "plain"}
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	service := testToolService(t)
	if _, err := BuildConversation(service, nil, NewAgents(coord), ConversationSpec{
		ProviderName: "test", ModelID: "m", ModelFactory: staticFactory{model: model},
	}); err != nil {
		t.Fatal(err)
	}
	if got := coord.LanguageModel(); got != sdk.LanguageModel(model) {
		t.Fatalf("coordinator model = %#v, want the same instance", got)
	}
}

// TestBindRootMemoryIsANoOpForPlainFactory verifies the rebind seam tolerates
// factories that do not own root memory.
func TestBindRootMemoryIsANoOpForPlainFactory(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("BindRootMemory panicked: %v", r)
		}
	}()
	BindRootMemory(staticFactory{model: &recordingModel{id: "plain"}}, "session", "ws")
	BindRootMemory(nil, "session", "ws")
	BindRootMemory(staticFactory{model: &recordingModel{id: "plain"}}, "session", "")
}
