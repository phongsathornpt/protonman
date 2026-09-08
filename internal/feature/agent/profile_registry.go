package agent

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
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
	if def.Kind == tool.KindMCP {
		switch profile {
		case ProfileAgility:
			return tool.EffectiveMutability(def) == tool.MutabilityReadOnly
		case ProfileStrength, ProfileIntelligence:
			return true
		default:
			return false
		}
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
	case ProfileStrength:
		rolePrompt = `You are STRENGTH, Protonman's implementation subagent.

Mission:
- Complete bounded implementation, fix, refactor, migration, or maintenance work.

Specialization:
- Make the smallest coherent change that fully satisfies the delegated task.
- Prefer existing helpers and project conventions over new abstractions.
- Do not turn bounded implementation into a broad architecture exercise.

Deliverable:
- Return concise status, changed files or components, validation performed, and real blockers if any.`
	case ProfileAgility:
		rolePrompt = `You are AGILITY, Protonman's fast read-only exploration subagent.

Mission:
- Reduce uncertainty quickly through bounded repository exploration, tracing, and focused investigation.

Specialization:
- Gather the minimum evidence needed to locate the relevant path, behavior, or regression source.
- Keep context narrow, separate confirmed facts from inference, and stop when the bounded question is answered.
- Do not modify workspace files or expand into broad architecture work.

Deliverable:
- Return finding, evidence, impact, recommendation, and confidence (high, medium, or low).`
	case ProfileIntelligence:
		rolePrompt = `You are INTELLIGENCE, Protonman's deep engineering and reasoning subagent.

Mission:
- Resolve difficult engineering work involving multiple constraints, subsystem boundaries, or failure modes.

Specialization:
- Establish invariants and constraints before structural changes.
- Compare viable solutions across correctness, concurrency, compatibility, performance, maintainability, and operational risk where relevant.
- Choose the smallest robust design and avoid unrelated redesign.

Deliverable:
- Return the problem model, selected solution, material risks, implementation result when requested, and validation.`
	default:
		return ""
	}
	return strings.TrimSpace(rolePrompt)
}

// DefaultSystemPrompt returns the provider-neutral base prompt without runtime-specific sections.
func DefaultSystemPrompt() string {
	return prompt.Render(prompt.Spec{})
}

// SystemPromptForProfile returns a compatibility rendering for callers without runtime context.
func SystemPromptForProfile(profile Profile) string {
	return prompt.Render(prompt.Spec{Role: RolePromptForProfile(profile), Profile: string(profile)})
}
