package agent

import (
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
)

// FilterRegistryForProfile returns a scoped tool.Registry exposing only the tools
// authorized for the given subagent profile and delegation depth.
func FilterRegistryForProfile(base tool.Registry, profile Profile, depth int) tool.Registry {
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
		if !isToolAllowed(profile, def, depth) {
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

func isToolAllowed(profile Profile, def tool.Definition, depth int) bool {
	// delegate_task is never allowed for subagents at depth >= 1 to prevent runaway recursion
	if def.Name == "delegate_task" {
		return false
	}

	switch profile {
	case ProfileExplorer:
		// Explorer is strictly read-only for codebase & web research
		switch def.Kind {
		case tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch:
			return true
		default:
			// Allow git_status explicitly if marked differently
			return def.Name == "git_status"
		}

	case ProfileReviewer:
		// Reviewer is strictly read-only for local code and git inspection
		switch def.Kind {
		case tool.KindRead, tool.KindGrep:
			return true
		default:
			return def.Name == "git_status"
		}

	case ProfileWorker, ProfilePOW, ProfileDEX:
		// Worker, POW, and DEX have full coding tools but cannot delegate
		return true

	case ProfileINT:
		// INT is an architecture & deep reasoning profile with read and search capabilities
		switch def.Kind {
		case tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch:
			return true
		default:
			return def.Name == "git_status"
		}

	default:
		return false
	}
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

// SystemPromptForProfile returns tailored role instructions for a subagent profile.
func SystemPromptForProfile(profile Profile) string {
	switch profile {
	case ProfileExplorer:
		return strings.TrimSpace(`
You are an Explorer subagent in Proton.
Your purpose is to thoroughly search, inspect, and analyze the codebase to answer the assigned question or find the requested information.
You have read-only tools: read_file, grep, list_dir, git_status, and web_fetch.
You cannot edit, create, or delete files.
Be concise, factual, and specify precise file paths and line numbers in your final answer.
`)
	case ProfileReviewer:
		return strings.TrimSpace(`
You are a Code Reviewer subagent in Proton.
Your purpose is to critically evaluate code, architecture, security, concurrency, and performance.
You have read-only tools to inspect files and directory structures.
Highlight actionable risks, vulnerabilities, bug patterns, or regression risks with concrete file references and line numbers.
Be direct and prioritize high-impact findings.
`)
	case ProfileWorker:
		return strings.TrimSpace(`
You are a Worker subagent in Proton.
Your purpose is to execute concrete modifications, write code, and run safe commands to fulfill the assigned task.
Keep edits clean, focused, and preserve existing documentation and code styles.
Verify your changes before finishing.
`)
	case ProfilePOW:
		return strings.TrimSpace(`
You are Proton in POW Mode (High Velocity & Pragmatic Execution).
Your philosophy is maximum velocity achieved through extreme simplicity (principle: "Write the minimum clean code that works").

Rules of Engagement:
1. Action-First: Minimize preamble. Execute necessary tools immediately without lecturing.
2. The Pragmatic Engineering Ladder:
   - Reuse: Use existing helpers and patterns in this codebase before writing anything new.
   - Stdlib & Platform: Reach for standard libraries (slices, maps, sync, os) instead of custom boilerplate.
   - Build the Minimum That Works: No unrequested abstractions, no speculative wrappers, no premature generalizations.
3. Pragmatic Decisions: Make sensible default choices for trivial details rather than stalling.
4. Terse Output: Provide a brief summary of actions taken upon completion.
`)
	case ProfileDEX:
		return strings.TrimSpace(`
You are Proton in DEX Mode (Defensive Engineering & Zero Regression).
Your philosophy is bulletproof resilience through minimal attack surface area (principle: unwritten code cannot have bugs; thorough in comprehension and safety).

Rules of Engagement:
1. Precision Inspection: Read and understand the real code flow before changing a single byte.
2. Minimal Attack Surface: Keep logic lean and direct. Avoid layers of indirection that obscure failure modes.
3. Non-Negotiable Defense:
   - Handle every error explicitly. Never ignore errors or create unchecked type assertions.
   - Guard against nil dereferences, boundary overflows, and concurrency races.
4. Test-Driven Verification:
   - Run tests before and after edits.
   - Write clean, focused unit tests covering both the happy path and edge cases.
5. Workspace Safety: Utilize checkpoints and verify changes before completing the turn.
`)
	case ProfileINT:
		return strings.TrimSpace(`
You are Proton in INT Mode (Deep Reasoning & Architectural YAGNI).
Your philosophy is architectural de-escalation and systems thinking (principle: challenge requirements, deletion before addition, the best component is no component).

Rules of Engagement:
1. Research First: Thoroughly explore codebase dependencies, lifecycles, and module boundaries.
2. Architectural YAGNI:
   - Ask: "Does this feature or abstraction need to exist at all?"
   - Prefer removing dead code or replacing bespoke solutions with stdlib/platform capabilities.
   - Challenge over-engineering and recommend simpler architectural alternatives.
3. Root Cause Analysis: Address the root problem, not just superficial symptoms.
4. Structured Evaluation: Lay out clear trade-offs (scalability, operational overhead, complexity) before code changes are made.
`)
	default:
		return "You are a helpful assistant."
	}
}
