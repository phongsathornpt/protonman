package turn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type goalTodoHandler struct {
	payload json.RawMessage
	calls   int
}

func (h *goalTodoHandler) Definition() tool.Definition {
	return tool.Definition{Name: tool.NameTodo, Description: "read task plan", Kind: tool.KindTask, Mutability: tool.MutabilityReadOnly}
}

func (h *goalTodoHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: "task snapshot", StructuredOutput: append(json.RawMessage(nil), h.payload...)}, nil
}

func newGoalTodoLoop(t *testing.T, client sdk.LanguageModel, payload string) (*Loop, *goalTodoHandler) {
	t.Helper()
	handler := &goalTodoHandler{payload: json.RawMessage(payload)}
	policy, err := permission.NewPolicy(permission.Config{Rules: []permission.Rule{{Action: permission.ActionAllow, Tool: permission.ToolTask}}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(&recordingRegistry{handler: handler}, policy)
	if err != nil {
		t.Fatal(err)
	}
	loop, err := NewLoop(client, service, WithSystemPromptSpec(prompt.Spec{ActiveGoal: "finish the tracked work"}))
	if err != nil {
		t.Fatal(err)
	}
	return loop, handler
}

func TestLoopKeepsActiveGoalOpenWhenTrackedPlanIsIncomplete(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{{Kind: sdk.EventToolCall, ToolCall: sdk.ToolCall{ID: "todo-1", Name: tool.NameTodo, Arguments: json.RawMessage(`{"action":"get"}`)}}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "I need another turn."}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	loop, _ := newGoalTodoLoop(t, client, `{"revision":3,"items":[{"id":"a","text":"first","status":"completed"},{"id":"b","text":"second","status":"pending"}]}`)
	result, err := loop.Run(context.Background(), []sdk.Message{{Role: sdk.RoleUser, Content: "continue"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.GoalCompleted {
		t.Fatal("incomplete tracked plan completed the active goal")
	}
	request := client.requests[1]
	found := false
	for _, message := range request.Messages {
		if message.Role == sdk.RoleSystem && strings.Contains(message.Content, "ACTIVE GOAL PROGRESS") && strings.Contains(message.Content, "1 pending") {
			found = true
		}
	}
	if !found {
		t.Fatalf("follow-up request missing active-goal progress guard: %#v", request.Messages)
	}
}

func TestLoopCompletesActiveGoalWhenTrackedPlanIsComplete(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{{Kind: sdk.EventToolCall, ToolCall: sdk.ToolCall{ID: "todo-1", Name: tool.NameTodo, Arguments: json.RawMessage(`{"action":"get"}`)}}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "Done."}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	loop, _ := newGoalTodoLoop(t, client, `{"revision":4,"items":[{"id":"a","text":"first","status":"completed"}]}`)
	result, err := loop.Run(context.Background(), []sdk.Message{{Role: sdk.RoleUser, Content: "finish"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.GoalCompleted {
		t.Fatal("completed tracked plan did not complete the active goal")
	}
}

func TestLoopCompletesActiveGoalFromInitialCompletedTaskPlanWithoutCallingTodo(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "Done."}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	loop, _ := newGoalTodoLoop(t, client, `{"revision":4,"items":[{"id":"a","text":"first","status":"completed"}]}`)
	result, err := loop.Run(context.Background(), []sdk.Message{{Role: sdk.RoleUser, Content: "finish"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.GoalCompleted {
		t.Fatal("already-completed task plan did not complete the active goal when turn needed no todo calls")
	}
}

func TestGoalCompletionRequiresVerificationAfterMutation(t *testing.T) {
	loop := &Loop{promptSpec: &prompt.Spec{ActiveGoal: "ship change"}}
	plan := taskPlanProgress{observed: true, total: 1, completed: 1}
	if loop.goalCompleted(plan, VerificationState{Mutated: true}) {
		t.Fatal("unverified mutation completed active goal")
	}
	if !loop.goalCompleted(plan, VerificationState{Mutated: true, Verified: true, Verifier: "go test"}) {
		t.Fatal("verified completed plan did not complete active goal")
	}
}
