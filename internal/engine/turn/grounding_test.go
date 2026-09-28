package turn

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	port "github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
)

type groundingHandler struct {
	definition tool.Definition
	fail       bool
}

func (h groundingHandler) Definition() tool.Definition { return h.definition }
func (h groundingHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	result := tool.Result{CallID: call.ID, ToolName: call.Name, Output: "observed"}
	if h.fail {
		err := errors.New("observation failed")
		result.Failure = tool.FailureFromError(err)
		return result, err
	}
	return result, nil
}

type groundingRegistry map[string]tool.Handler

func (r groundingRegistry) Lookup(name string) (tool.Handler, bool) {
	h, ok := r[name]
	return h, ok
}
func (r groundingRegistry) Definitions() []tool.Definition {
	out := make([]tool.Definition, 0, len(r))
	for _, h := range r {
		out = append(out, h.Definition())
	}
	return out
}

func newGroundingLoop(t *testing.T, client port.LanguageModel, handlers groundingRegistry, options ...Option) *Loop {
	t.Helper()
	policy, err := permission.NewPolicy(permission.Config{Default: permission.ActionAllow})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(handlers, policy, toolcall.WithMode(permission.ModeAlwaysApprove))
	if err != nil {
		t.Fatal(err)
	}
	loop, err := NewLoop(client, service, options...)
	if err != nil {
		t.Fatal(err)
	}
	return loop
}

func TestGroundingRestrictsToolsUntilSuccessfulWorkspaceEvidence(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []domain.Event{
			{Kind: domain.EventToolCall, ToolCall: model.ToolCall{ID: "read-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
			{Kind: domain.EventFinish, FinishReason: domain.FinishToolCalls},
		}},
		{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "done"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	client.profile.Capabilities.ToolChoiceRequired = modelprofile.SupportYes
	loop := newGroundingLoop(t, client, groundingRegistry{
		"read": groundingHandler{definition: tool.Definition{Name: "read", Description: "read", Kind: tool.KindRead, Evidence: tool.EvidenceWorkspace}},
		"todo": groundingHandler{definition: tool.Definition{Name: "todo", Description: "tasks", Kind: tool.KindTask}},
		"bash": groundingHandler{definition: tool.Definition{Name: "bash", Description: "shell", Kind: tool.KindBash, Mutability: tool.MutabilityMutating}},
	}, WithGroundingEvidence(tool.EvidenceWorkspace))

	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect repo"}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
	if got := toolNames(client.requests[0].Tools); len(got) != 1 || got[0] != "read" {
		t.Fatalf("grounding tools = %#v, want only read", got)
	}
	if client.requests[0].Options.ToolChoice != domain.ToolChoiceRequired {
		t.Fatalf("grounding tool choice = %q, want required", client.requests[0].Options.ToolChoice)
	}
	if got := toolNames(client.requests[1].Tools); !containsTool(got, "todo") || !containsTool(got, "bash") || !containsTool(got, "read") {
		t.Fatalf("post-grounding tools = %#v, want full registry", got)
	}
	if client.requests[1].Options.ToolChoice != domain.ToolChoiceAuto {
		t.Fatalf("post-grounding tool choice = %q, want auto", client.requests[1].Options.ToolChoice)
	}
}

func TestGroundingDefersFinalTextWhenProviderIgnoresRequiredToolChoice(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "I think it is fine"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
		{events: []domain.Event{
			{Kind: domain.EventToolCall, ToolCall: model.ToolCall{ID: "read-2", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
			{Kind: domain.EventFinish, FinishReason: domain.FinishToolCalls},
		}},
		{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "grounded answer"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	client.profile.Capabilities.ToolChoiceRequired = modelprofile.SupportYes
	loop := newGroundingLoop(t, client, groundingRegistry{
		"read": groundingHandler{definition: tool.Definition{Name: "read", Description: "read", Kind: tool.KindRead, Evidence: tool.EvidenceWorkspace}},
	}, WithGroundingEvidence(tool.EvidenceWorkspace))

	result, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "grounded answer" {
		t.Fatalf("final answer = %q", result.Message.Content)
	}
	if len(client.requests) != 3 || client.requests[0].Options.ToolChoice != domain.ToolChoiceRequired || client.requests[1].Options.ToolChoice != domain.ToolChoiceRequired || client.requests[2].Options.ToolChoice != domain.ToolChoiceAuto {
		t.Fatalf("tool choices = %#v", client.requests)
	}
}

func TestGroundingUsesAutoWhenRequiredToolChoiceIsUnknown(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []domain.Event{
			{Kind: domain.EventToolCall, ToolCall: model.ToolCall{ID: "read-auto", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
			{Kind: domain.EventFinish, FinishReason: domain.FinishToolCalls},
		}},
		{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "done"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	loop := newGroundingLoop(t, client, groundingRegistry{
		"read": groundingHandler{definition: tool.Definition{Name: "read", Description: "read", Kind: tool.KindRead, Evidence: tool.EvidenceWorkspace}},
	}, WithGroundingEvidence(tool.EvidenceWorkspace))
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0].Options.ToolChoice; got != domain.ToolChoiceAuto {
		t.Fatalf("grounding tool choice = %q, want auto for unknown capability", got)
	}
}

func TestGroundingStopsAfterRepeatedUngroundedFinalResponses(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "guess one"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
		{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "guess two"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	loop := newGroundingLoop(t, client, groundingRegistry{
		"read": groundingHandler{definition: tool.Definition{Name: "read", Description: "read", Kind: tool.KindRead, Evidence: tool.EvidenceWorkspace}},
	}, WithGroundingEvidence(tool.EvidenceWorkspace))
	_, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil)
	if !errors.Is(err, ErrGroundingUnavailable) {
		t.Fatalf("Run() error = %v, want grounding unavailable", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
}

func TestGroundingFailedEvidenceDoesNotSatisfyState(t *testing.T) {
	state := newGroundingState(tool.EvidenceWorkspace)
	definitions := []tool.Definition{{Name: "read", Description: "read", Kind: tool.KindRead, Evidence: tool.EvidenceWorkspace}}
	executions := []executedCall{{
		call:   tool.Call{ID: "read", Name: "read"},
		result: tool.Result{CallID: "read", ToolName: "read", Failure: &tool.Failure{Code: tool.ErrorCodeExecution, Message: "failed"}},
		err:    errors.New("failed"),
	}}
	if state.observe(executions, definitions) || !state.pending() {
		t.Fatal("failed evidence unexpectedly satisfied grounding")
	}
}

func TestGroundingRequiresAvailableEvidenceTools(t *testing.T) {
	client := &scriptedClient{}
	loop := newGroundingLoop(t, client, groundingRegistry{
		"todo": groundingHandler{definition: tool.Definition{Name: "todo", Description: "tasks", Kind: tool.KindTask}},
	}, WithGroundingEvidence(tool.EvidenceWorkspace))
	_, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil)
	if !errors.Is(err, ErrGroundingUnavailable) {
		t.Fatalf("Run() error = %v, want grounding unavailable", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want 0", len(client.requests))
	}
}

func toolNames(tools []domain.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, item := range tools {
		out = append(out, item.Name)
	}
	return out
}

func containsTool(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
