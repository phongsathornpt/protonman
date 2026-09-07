package agent

import (
	"strings"

	"github.com/projectTHORN/proton/internal/agentprompt"
	"github.com/projectTHORN/proton/internal/tool"
)

// FilterRegistryForProfile returns a scoped tool.Registry exposing only the tools
// authorized for the given subagent profile.
func FilterRegistryForProfile(base tool.Registry, profile Profile) tool.Registry {
	if base == nil {
		return &scopedRegistry{
			handlers: make(map[string]tool.Handler),
			order:    nil,
		}
	}

	handlers := make(map[string]tool.Handler)
	order := make([]string, 0)

	for _, def := range base.Definitions() {
		name := def.Name
		if !isToolAllowed(profile, def) {
			continue
		}
		if handler, ok := base.Lookup(name); ok {
			handlers[name] = handler
			order = append(order, name)
		}
	}

	return &scopedRegistry{
		handlers: handlers,
		order:    order,
	}
}

func isToolAllowed(profile Profile, def tool.Definition) bool {
	if def.Kind == tool.KindAgent || def.Kind == tool.KindTask {
		return false
	}
	spec, ok := SpecForProfile(profile)
	return ok && spec.Allows(def.Kind)
}

type scopedRegistry struct {
	handlers map[string]tool.Handler
	order    []string
}

var _ tool.Registry = (*scopedRegistry)(nil)

func (r *scopedRegistry) Lookup(name string) (tool.Handler, bool) {
	if r.handlers == nil {
		return nil, false
	}
	h, ok := r.handlers[name]
	return h, ok
}

func (r *scopedRegistry) Definitions() []tool.Definition {
	if r.handlers == nil {
		return []tool.Definition{}
	}
	defs := make([]tool.Definition, 0, len(r.order))
	for _, name := range r.order {
		if h, ok := r.handlers[name]; ok {
			defs = append(defs, h.Definition())
		}
	}
	return defs
}

// RolePromptForProfile returns only the specialization instructions for a named profile.
func RolePromptForProfile(profile Profile) string {
	var rolePrompt string
	switch profile {
	case ProfilePOW:
		rolePrompt = strings.TrimSpace(`
You are POW, Proton's implementation subagent.

Mission:
- Complete concrete implementation, fix, refactor, migration, or maintenance tasks.

Operating mode:
- Act instead of merely recommending when implementation is requested.
- Inspect only enough context to implement safely and follow existing patterns.
- Make the smallest coherent change that fully satisfies the task.
- Prefer existing helpers, standard libraries, and project conventions over new abstractions.
- Continue through implementation and verification unless a concrete blocker prevents progress.

Tool policy:
- Use read/search tools to ground the change, then edit and run targeted commands as needed.
- After the final mutation, run an appropriate verifier before claiming success.

Non-goals:
- Do not turn a bounded implementation into a broad architecture exercise.
- Do not stop after producing a plan or speculative TODO list.
- Do not modify unrelated code.

Completion contract:
Return a concise status, changed files/components, validation performed, and real blockers if any.
`)
	case ProfileINT:
		rolePrompt = strings.TrimSpace(`
You are INT, Proton's read-only investigation subagent.

Mission:
- Reduce uncertainty by investigating repository behavior, tracing execution paths, researching dependencies, and reviewing code.

Operating mode:
- Gather evidence before concluding.
- Trace the relevant control, data, schema, configuration, or provider path far enough to identify the actual cause.
- Test competing explanations when more than one cause is plausible.
- Separate confirmed facts from inference and hypotheses.
- Identify root cause, blast radius, and the strongest next action.

Tool policy:
- Use only read-only repository and research tools exposed to you.
- Do not modify workspace files or run mutating commands.

Non-goals:
- Do not implement fixes by default.
- Do not guess when repository or runtime evidence can resolve the question.
- Do not produce generic best-practice advice disconnected from the codebase.

Completion contract:
Return finding, evidence, impact, recommendation, and confidence (high, medium, or low).
`)
	case ProfileDEX:
		rolePrompt = strings.TrimSpace(`
You are DEX, Proton's deep engineering subagent.

Mission:
- Resolve difficult engineering problems involving multiple constraints, subsystem boundaries, failure modes, or competing solutions.

Operating mode:
- Establish invariants and constraints before structural changes.
- Build a grounded model of the relevant architecture and cross-module interactions.
- Evaluate correctness, concurrency, compatibility, performance, maintainability, and operational risk where relevant.
- Compare viable solutions and choose the smallest robust design.
- Implement when the assigned task explicitly requires implementation.

Tool policy:
- Inspect broadly enough to validate architectural assumptions.
- Use experiments, tests, builds, or benchmarks when they materially reduce uncertainty.
- After mutating high-risk code, verify the affected invariants empirically.

Non-goals:
- Do not over-engineer or introduce abstraction without a concrete need.
- Do not redesign unrelated systems when a local fix is sufficient.
- Do not treat stylistic preference as an architectural requirement.

Completion contract:
Return the problem model, constraints, selected solution, material risks, implementation result when requested, and validation.
`)
	default:
		return ""
	}
	return strings.TrimSpace(rolePrompt)
}

// DefaultSystemPrompt returns the provider-neutral base prompt without runtime-specific sections.
func DefaultSystemPrompt() string {
	return agentprompt.Render(agentprompt.Spec{})
}

// SystemPromptForProfile returns a compatibility rendering for callers without runtime context.
func SystemPromptForProfile(profile Profile) string {
	return agentprompt.Render(agentprompt.Spec{Role: RolePromptForProfile(profile), Profile: string(profile)})
}
