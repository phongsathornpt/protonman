package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
)

func (c *Coordinator) execute(ctx context.Context, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{AgentID: req.ID, Profile: req.Profile}, err
	}

	c.agentsMu.RLock()
	parentRegistry := c.parentRegistry
	languageModel := c.languageModel
	permMode := c.permissionMode
	prompt := c.prompt
	guard := c.guard
	c.agentsMu.RUnlock()

	// 1. Build profile-scoped tool registry
	scopedRegistry := FilterRegistryForProfile(parentRegistry, req.Profile, req.Depth)

	// 2. Build scoped tool service
	// For workers: if parent mode is always-approve, inherit always-approve.
	// Otherwise, run in parent mode (or ModeAuto by default) and attach prompt.
	// For read-only: uses always-approve mode since tools are already restricted to safe reads.
	serviceMode := permission.ModeAlwaysApprove
	if req.Profile.IsMutating() {
		if permMode == permission.ModeAlwaysApprove {
			serviceMode = permission.ModeAlwaysApprove
		} else if permMode.Valid() {
			serviceMode = permMode
		} else {
			serviceMode = permission.ModeAuto
		}
	}

	policy := c.policy
	if policy == nil {
		p, err := permission.NewPolicy(permission.Config{})
		if err != nil {
			return Result{AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create default policy: %w", err)
		}
		policy = p
	}

	serviceOpts := []toolcall.Option{toolcall.WithMode(serviceMode)}
	if prompt != nil {
		serviceOpts = append(serviceOpts, toolcall.WithPrompt(prompt))
	}

	service, err := toolcall.NewService(
		scopedRegistry,
		policy,
		serviceOpts...,
	)
	if err != nil {
		return Result{AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create scoped tool service: %w", err)
	}
	if guard != nil {
		service.SetCallGuard(guard)
	}

	// 3. Resolve turn runner
	var runner turn.Runner
	if c.runnerFactory != nil {
		r, rerr := c.runnerFactory(req.Profile, service)
		if rerr != nil {
			return Result{AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create turn runner: %w", rerr)
		}
		runner = r
	} else {
		if languageModel == nil {
			return Result{AgentID: req.ID, Profile: req.Profile}, errors.New("language model is required for subagent execution")
		}
		loop, lerr := turn.NewLanguageModelLoop(
			languageModel,
			service,
			turn.WithMaxRounds(c.maxRounds),
			turn.WithMaxToolCalls(c.maxToolCalls),
		)
		if lerr != nil {
			return Result{AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create turn loop: %w", lerr)
		}
		runner = loop
	}

	// 4. Build messages
	systemContent := SystemPromptForProfile(req.Profile)
	messages := []model.Message{
		{Role: model.RoleSystem, Content: systemContent},
		{Role: model.RoleUser, Content: formatUserPrompt(req)},
	}

	// 5. Run turn
	turnResult, err := runner.Run(ctx, messages, func(_ context.Context, te turn.Event) error {
		if te.Kind == turn.EventToolCall {
			c.emit(ctx, Event{
				Kind:     EventAgentProgress,
				AgentID:  req.ID,
				ParentID: req.ParentID,
				Profile:  req.Profile,
				Message:  fmt.Sprintf("using %s", te.Call.Name),
			})
		}
		return nil
	})
	if err != nil {
		return Result{AgentID: req.ID, Profile: req.Profile, Rounds: turnResult.Rounds}, err
	}

	summary := strings.TrimSpace(turnResult.Message.Content)
	if summary == "" {
		summary = "Task completed with no final text response."
	}
	if len(summary) > maxSummaryBytes {
		summary = summary[:maxSummaryBytes] + "\n... [output truncated]"
	}

	return Result{
		AgentID: req.ID,
		Profile: req.Profile,
		Summary: summary,
		Rounds:  turnResult.Rounds,
	}, nil
}

func formatUserPrompt(req Request) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Task: %s\n", req.Task))
	if req.Context != "" {
		b.WriteString(fmt.Sprintf("\nContext:\n%s\n", req.Context))
	}
	return b.String()
}
