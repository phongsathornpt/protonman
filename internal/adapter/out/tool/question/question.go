package questiontool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type askQuestionHandler struct {
	prompter Prompter
}

// NewAskQuestion creates a new ask_question tool handler.
func NewAskQuestion(prompter Prompter) tool.Handler {
	return &askQuestionHandler{prompter: prompter}
}

func (h *askQuestionHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        tool.NameAskQuestion,
		Description: "Ask the user an interactive clarifying question, gather design preferences, or resolve ambiguous requirements. You MUST use this tool instead of asking questions in conversational assistant text.",
		Kind:        tool.KindQuestion,
		Mutability:  tool.MutabilityReadOnly,
		Safety: tool.SafetyContract{
			MutationDomain:   tool.MutationDomainNone,
			MutationSafety:   tool.MutationSafetyNone,
			CheckpointPolicy: tool.CheckpointPolicyNone,
			Boundary:         tool.BoundaryPolicyNone,
		},
		InputSchema:  inputSchema(),
		OutputSchema: outputSchema(),
	}
}

func (h *askQuestionHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if ctx.Err() != nil {
		return tool.Result{}, ctx.Err()
	}
	var req Request
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &req); err != nil {
			return tool.Result{}, fmt.Errorf("decode question arguments: %w", err)
		}
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		return tool.Result{}, errors.New("question must not be empty")
	}
	if len(req.Options) > 0 {
		cleaned := make([]string, 0, len(req.Options))
		seen := make(map[string]struct{}, len(req.Options))
		for _, opt := range req.Options {
			opt = strings.TrimSpace(opt)
			if opt == "" {
				continue
			}
			if _, exists := seen[opt]; !exists {
				seen[opt] = struct{}{}
				cleaned = append(cleaned, opt)
			}
		}
		req.Options = cleaned
	}

	if h.prompter == nil {
		resp := Response{
			Status: StatusDeclined,
			Answer: "User declined to answer (non-interactive mode)",
		}
		raw, _ := json.Marshal(resp)
		return tool.Result{
			CallID:           call.ID,
			ToolName:         call.Name,
			Output:           resp.Answer,
			StructuredOutput: raw,
		}, nil
	}

	resp, err := h.prompter.PromptQuestion(ctx, req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return tool.Result{}, err
		}
		resp = Response{
			Status: StatusDeclined,
			Answer: fmt.Sprintf("Question dismissed: %v", err),
		}
	}

	raw, jsonErr := json.Marshal(resp)
	if jsonErr != nil {
		return tool.Result{}, fmt.Errorf("marshal question response: %w", jsonErr)
	}

	output := resp.Answer
	if resp.Status == StatusDeclined && !strings.Contains(strings.ToLower(output), "declined") {
		output = "[Declined] " + output
	}

	return tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           output,
		StructuredOutput: raw,
	}, nil
}
