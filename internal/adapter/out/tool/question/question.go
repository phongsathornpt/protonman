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

// NewAskQuestion creates a new askQuestion tool handler.
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

	items := req.NormalizedItems()
	if len(items) == 0 {
		return tool.Result{}, errors.New("question must not be empty")
	}

	for i := range items {
		items[i].Question = strings.TrimSpace(items[i].Question)
		if items[i].Question == "" {
			return tool.Result{}, fmt.Errorf("question at index %d must not be empty", i)
		}
		if len(items[i].Options) > 0 {
			cleaned := make([]string, 0, len(items[i].Options))
			seen := make(map[string]struct{}, len(items[i].Options))
			for _, opt := range items[i].Options {
				opt = strings.TrimSpace(opt)
				if opt == "" {
					continue
				}
				if _, exists := seen[opt]; !exists {
					seen[opt] = struct{}{}
					cleaned = append(cleaned, opt)
				}
			}
			items[i].Options = cleaned
		}
	}
	req.Questions = items
	if len(items) == 1 {
		req.Question = items[0].Question
		req.Options = items[0].Options
		req.Multiple = items[0].Multiple
		req.Recommended = items[0].Recommended
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

	if resp.Answer == "" && len(resp.Answers) > 0 {
		if len(resp.Answers) == 1 {
			resp.Answer = resp.Answers[0].Answer
		} else {
			var sb strings.Builder
			for i, a := range resp.Answers {
				if i > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("%d. %s: %s", i+1, a.Question, a.Answer))
			}
			resp.Answer = sb.String()
		}
	}
	if len(resp.SelectedOptions) == 0 && len(resp.Answers) == 1 {
		resp.SelectedOptions = resp.Answers[0].SelectedOptions
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
