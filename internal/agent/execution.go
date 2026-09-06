package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

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
	scopedRegistry := FilterRegistryForProfile(parentRegistry, req.Profile)

	// 2. Build scoped tool service. Child agents inherit the parent permission
	// mode regardless of profile. Capability scoping limits which tools a
	// profile can see, but it must not silently upgrade ask/auto to
	// always-approve for network or other policy-sensitive reads.
	serviceMode := permMode
	if !serviceMode.Valid() {
		serviceMode = permission.ModeAuto
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
	if c.workspace != nil {
		serviceOpts = append(serviceOpts, toolcall.WithWorkspaceMutationGate(c.workspace))
	}
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
		loop, lerr := turn.NewLoop(
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
		return Result{AgentID: req.ID, Profile: req.Profile, Rounds: turnResult.Rounds, Verification: turnResult.Verification}, err
	}
	if req.Profile == ProfileDEX && turnResult.Verification.Mutated && !turnResult.Verification.Verified {
		return Result{AgentID: req.ID, Profile: req.Profile, Rounds: turnResult.Rounds, Verification: turnResult.Verification}, ErrUnverifiedChanges
	}

	summary := strings.TrimSpace(turnResult.Message.Content)
	if summary == "" {
		summary = "Task completed with no final text response."
	}
	if (req.Profile == ProfileWorker || req.Profile == ProfilePOW) && turnResult.Verification.Mutated && !turnResult.Verification.Verified {
		summary += "\n\nWarning: changes were not verified after the final mutation."
	}
	summary = truncateSummary(summary, maxSummaryBytes)

	return Result{
		AgentID:      req.ID,
		Profile:      req.Profile,
		Summary:      summary,
		Rounds:       turnResult.Rounds,
		Verification: turnResult.Verification,
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

func truncateSummary(summary string, maxBytes int) string {
	const suffix = "\n... [output truncated]"
	if maxBytes <= 0 {
		return ""
	}
	summary = strings.ToValidUTF8(summary, "�")
	if len(summary) <= maxBytes {
		return summary
	}
	if maxBytes <= len(suffix) {
		return suffix[:maxBytes]
	}
	cut := maxBytes - len(suffix)
	for cut > 0 && !utf8.ValidString(summary[:cut]) {
		cut--
	}
	return summary[:cut] + suffix
}
