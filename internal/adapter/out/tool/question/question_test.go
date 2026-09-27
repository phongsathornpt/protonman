package questiontool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type mockPrompter struct {
	response Response
	err      error
	called   bool
	gotReq   Request
}

func (m *mockPrompter) PromptQuestion(ctx context.Context, req Request) (Response, error) {
	m.called = true
	m.gotReq = req
	if ctx.Err() != nil {
		return Response{}, ctx.Err()
	}
	return m.response, m.err
}

func TestAskQuestionDefinition(t *testing.T) {
	h := NewAskQuestion(nil)
	def := h.Definition()
	if def.Name != tool.NameAskQuestion {
		t.Fatalf("def.Name = %q, want %q", def.Name, tool.NameAskQuestion)
	}
	if def.Kind != tool.KindQuestion {
		t.Fatalf("def.Kind = %q, want %q", def.Kind, tool.KindQuestion)
	}
	if def.Mutability != tool.MutabilityReadOnly {
		t.Fatalf("def.Mutability = %v, want read-only", def.Mutability)
	}
	if !strings.Contains(def.Description, "MUST use this tool instead of asking questions in conversational assistant text") {
		t.Fatalf("def.Description missing directive: %s", def.Description)
	}
}

func TestAskQuestionExecuteSuccess(t *testing.T) {
	prompter := &mockPrompter{
		response: Response{
			Status:          StatusAnswered,
			Answer:          "PostgreSQL",
			SelectedOptions: []string{"PostgreSQL"},
		},
	}
	h := NewAskQuestion(prompter)
	args := json.RawMessage(`{"question":"Which DB?","options":["PostgreSQL","SQLite"],"multiple":false}`)
	res, err := h.Execute(context.Background(), tool.Call{ID: "call-1", Name: "ask_question", Arguments: args})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !prompter.called {
		t.Fatal("expected prompter to be called")
	}
	if prompter.gotReq.Question != "Which DB?" {
		t.Fatalf("got question %q, want %q", prompter.gotReq.Question, "Which DB?")
	}
	if len(prompter.gotReq.Options) != 2 {
		t.Fatalf("got %d options, want 2", len(prompter.gotReq.Options))
	}
	if res.Output != "PostgreSQL" {
		t.Fatalf("res.Output = %q, want %q", res.Output, "PostgreSQL")
	}

	var structured Response
	if err := json.Unmarshal(res.StructuredOutput, &structured); err != nil {
		t.Fatalf("unmarshal structured output: %v", err)
	}
	if structured.Status != StatusAnswered || structured.Answer != "PostgreSQL" {
		t.Fatalf("unexpected structured output: %+v", structured)
	}
}

func TestAskQuestionExecuteEmptyQuestion(t *testing.T) {
	h := NewAskQuestion(&mockPrompter{})
	_, err := h.Execute(context.Background(), tool.Call{ID: "call-1", Name: "ask_question", Arguments: json.RawMessage(`{"question":""}`)})
	if err == nil {
		t.Fatal("expected error for empty question, got nil")
	}
}

func TestAskQuestionExecuteNilPrompter(t *testing.T) {
	h := NewAskQuestion(nil)
	res, err := h.Execute(context.Background(), tool.Call{ID: "call-1", Name: "ask_question", Arguments: json.RawMessage(`{"question":"hello?"}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var structured Response
	if err := json.Unmarshal(res.StructuredOutput, &structured); err != nil {
		t.Fatalf("unmarshal structured output: %v", err)
	}
	if structured.Status != StatusDeclined {
		t.Fatalf("status = %q, want %q", structured.Status, StatusDeclined)
	}
}

func TestAskQuestionExecuteContextCanceled(t *testing.T) {
	prompter := &mockPrompter{
		err: context.Canceled,
	}
	h := NewAskQuestion(prompter)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.Execute(ctx, tool.Call{ID: "call-1", Name: "ask_question", Arguments: json.RawMessage(`{"question":"hello?"}`)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestAskQuestionSanitizesAndDeduplicatesOptions(t *testing.T) {
	prompter := &mockPrompter{
		response: Response{
			Status: StatusAnswered,
			Answer: "PostgreSQL",
		},
	}
	h := NewAskQuestion(prompter)
	args := json.RawMessage(`{"question":"Which DB?","options":[" PostgreSQL ", "", "  ", "SQLite", "PostgreSQL"]}`)
	_, err := h.Execute(context.Background(), tool.Call{ID: "call-1", Name: "ask_question", Arguments: args})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompter.gotReq.Options) != 2 {
		t.Fatalf("got %d options, want 2: %+v", len(prompter.gotReq.Options), prompter.gotReq.Options)
	}
	if prompter.gotReq.Options[0] != "PostgreSQL" || prompter.gotReq.Options[1] != "SQLite" {
		t.Fatalf("unexpected options: %+v", prompter.gotReq.Options)
	}
}
