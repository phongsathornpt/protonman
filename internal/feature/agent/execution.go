package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func (c *Coordinator) execute(ctx context.Context, req Request) (Result, error) {
	c.agentsMu.RLock()
	languageModel := c.languageModel
	reasoningEffort := c.reasoningEffort
	runtimeSpec := childToolRuntime{
		permissionMode: c.permissionMode, prompt: c.prompt, guard: c.guard,
		permissionTimeout: c.toolPermissionTimeout, executionTimeout: c.toolExecutionTimeout, observer: c.toolObserver,
	}
	c.agentsMu.RUnlock()
	return c.executeWithRuntime(ctx, req, languageModel, reasoningEffort, runtimeSpec)
}

func (c *Coordinator) executeWithRuntime(ctx context.Context, req Request, languageModel sdk.LanguageModel, reasoningEffort sdk.ReasoningEffort, runtimeSpec childToolRuntime) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile}, err
	}

	c.agentsMu.RLock()
	parentRegistry := c.parentRegistry
	skillCatalog := c.skillRegistry
	c.agentsMu.RUnlock()
	permMode := runtimeSpec.permissionMode
	prompter := runtimeSpec.prompt
	guard := runtimeSpec.guard

	// 1. Build profile-scoped tools and an isolated skill activation session.
	childSkills := selectSubagentSkills(skillCatalog, req)
	scopedRegistry := FilterRegistryForProfile(parentRegistry, req.Profile)
	scopedRegistry = bindSkillRegistry(scopedRegistry, childSkills)

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
			return Result{SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create default policy: %w", err)
		}
		policy = p
	}

	serviceOpts := []toolcall.Option{
		toolcall.WithMode(serviceMode),
		toolcall.WithPermissionTimeout(runtimeSpec.permissionTimeout),
		toolcall.WithExecutionTimeout(runtimeSpec.executionTimeout),
	}
	if runtimeSpec.observer != nil {
		serviceOpts = append(serviceOpts, toolcall.WithObserver(runtimeSpec.observer))
	}
	if c.workspace != nil {
		serviceOpts = append(serviceOpts, toolcall.WithWorkspaceMutationGate(c.workspace))
	}
	if prompter != nil {
		serviceOpts = append(serviceOpts, toolcall.WithPrompt(prompter))
	}

	service, err := toolcall.NewService(
		scopedRegistry,
		policy,
		serviceOpts...,
	)
	if err != nil {
		return Result{SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create scoped tool service: %w", err)
	}
	if guard != nil {
		service.SetCallGuard(guard)
	}

	// 3. Resolve turn runner
	var runner turn.Runner
	if c.runnerFactory != nil {
		r, rerr := c.runnerFactory(req.Profile, service)
		if rerr != nil {
			return Result{SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create turn runner: %w", rerr)
		}
		runner = r
	} else {
		if languageModel == nil {
			return Result{SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile}, errors.New("language model is required for subagent execution")
		}
		promptSpec := prompt.Spec{Profile: string(req.Profile), Role: RolePromptForProfile(req.Profile)}
		if c.workspace != nil {
			promptSpec.Workspace = c.workspace.Root()
		}
		loopOptions := []turn.Option{
			turn.WithSystemPromptSpec(promptSpec),
			turn.WithMaxToolCalls(c.maxToolCalls),
			turn.WithWorkspacePolicy(c.workspace),
		}
		if spec, ok := SpecForProfile(req.Profile); ok {
			loopOptions = append(loopOptions, turn.WithGroundingEvidence(spec.GroundingEvidence), turn.WithReasoningEffort(spec.Reasoning))
		}
		if reasoningEffort != sdk.ReasoningDefault {
			loopOptions = append(loopOptions, turn.WithExplicitReasoningEffort(reasoningEffort))
		}
		if childSkills != nil {
			loopOptions = append(loopOptions, turn.WithSkillRegistry(childSkills))
		}
		loop, lerr := turn.NewLoop(languageModel, service, loopOptions...)
		if lerr != nil {
			return Result{SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create turn loop: %w", lerr)
		}
		runner = loop
	}

	// 4. Build messages. The turn loop owns the managed system prompt via
	// WithSystemPromptSpec; only user/task content enters history here.
	messages := []model.Message{
		{Role: model.RoleUser, Content: formatUserPrompt(req)},
	}

	// 5. Run turn and retain successful execution evidence for the parent.
	evidence := make([]EvidenceRef, 0)
	changedTargets := make([]string, 0)
	seenEvidence := make(map[string]struct{})
	seenChanges := make(map[string]struct{})
	turnResult, err := runner.Run(ctx, messages, func(_ context.Context, te turn.Event) error {
		switch te.Kind {
		case turn.EventToolCall:
			call := te.Call
			c.emit(ctx, Event{
				Kind:      EventAgentProgress,
				SessionID: req.SessionID,
				AgentID:   req.ID,
				ParentID:  req.ParentID,
				Profile:   req.Profile,
				Call:      &call,
			})
		case turn.EventToolResult:
			if te.Err != nil || te.Result.Denied || te.Result.Failure != nil {
				break
			}
			target := strings.TrimSpace(te.Call.Target())
			key := te.Call.Name + "\x00" + target
			if _, exists := seenEvidence[key]; !exists {
				seenEvidence[key] = struct{}{}
				evidence = append(evidence, EvidenceRef{Tool: te.Call.Name, Target: target})
			}
			if handler, ok := scopedRegistry.Lookup(te.Call.Name); ok {
				def := handler.Definition()
				workspaceDomain := def.Safety.MutationDomain == tool.MutationDomainWorkspace || def.Safety.MutationDomain == tool.MutationDomainWorkspacePolicy
				if workspaceDomain && tool.EffectiveCallMutability(def, te.Call.Arguments) == tool.MutabilityMutating {
					changed := target
					if changed == "" {
						changed = te.Call.Name
					}
					if _, exists := seenChanges[changed]; !exists {
						seenChanges[changed] = struct{}{}
						changedTargets = append(changedTargets, changed)
					}
				}
			}
		}
		return nil
	})
	base := Result{
		SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile,
		Rounds: turnResult.Rounds, Verification: turnResult.Verification,
		Evidence: evidence, ChangedTargets: changedTargets,
	}
	if err != nil {
		return base, err
	}

	conclusion, findings, blockers := parseChildSemanticResult(turnResult.Message.Content, evidence)
	base.Conclusion = conclusion
	base.Findings = findings
	base.Blockers = blockers
	base.Summary = conclusion

	if req.Profile == ProfileIntelligence && turnResult.Verification.Mutated && !turnResult.Verification.Verified {
		base.Blockers = append(base.Blockers, "changes were not verified after the final mutation")
		return base, ErrUnverifiedChanges
	}
	if req.Profile == ProfileStrength && turnResult.Verification.Mutated && !turnResult.Verification.Verified {
		const warning = "Warning: changes were not verified after the final mutation."
		base.Blockers = append(base.Blockers, "changes were not verified after the final mutation")
		base.Summary = truncateSummary(base.Conclusion+"\n\n"+warning, maxSummaryBytes)
	}

	return base, nil
}

type skillRegistryBinder interface {
	BindSkillRegistry(*skill.Registry) tool.Handler
}

func bindSkillRegistry(registry tool.Registry, skills *skill.Registry) tool.Registry {
	if registry == nil || skills == nil {
		return registry
	}
	scoped, ok := registry.(*scopedRegistry)
	if !ok {
		return registry
	}
	for name, handler := range scoped.handlers {
		if binder, ok := handler.(skillRegistryBinder); ok {
			scoped.handlers[name] = binder.BindSkillRegistry(skills)
		}
	}
	return scoped
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
