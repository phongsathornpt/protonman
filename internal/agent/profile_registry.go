package agent

import (
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
)

// FilterRegistryForProfile returns a scoped tool.Registry exposing only the tools
// authorized for the given subagent profile and delegation depth.
func FilterRegistryForProfile(base tool.Registry, profile Profile, _ int) tool.Registry {
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
	// Parent-owned orchestration/task state never crosses into subagents.
	// Keep this fail-closed so future agent/task tools do not silently escape
	// into child capability scopes.
	if def.Kind == tool.KindAgent || def.Kind == tool.KindTask {
		return false
	}

	switch profile {
	case ProfileExplorer:
		// Explorer is strictly read-only for codebase & web research.
		switch def.Kind {
		case tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch:
			return true
		default:
			return false
		}

	case ProfileReviewer:
		// Reviewer is strictly read-only for local code and git inspection.
		switch def.Kind {
		case tool.KindRead, tool.KindGrep:
			return true
		default:
			return false
		}

	case ProfileWorker, ProfilePOW, ProfileDEX:
		// Mutating profiles are still an explicit allowlist. New tool kinds are
		// denied until consciously added here rather than inheriting authority.
		switch def.Kind {
		case tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch, tool.KindEdit, tool.KindBash:
			return true
		default:
			return false
		}

	case ProfileINT:
		// INT is an architecture & deep reasoning profile with read and search capabilities.
		switch def.Kind {
		case tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch:
			return true
		default:
			return false
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
You have read-only tools: read_file, grep, list_dir, git_status, web_fetch, and web_search when available.
You cannot edit, create, delete files, manage the parent task plan, or orchestrate other subagents.
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
Your philosophy is maximum velocity achieved through extreme simplicity and capacity-limited execution (principle: "Write the minimum clean code that works").

Cognitive Architecture & Working Memory:
- Capacity Limit = 1: Keep strictly ONE active micro-goal on stage at any instant. Avoid multi-clause speculative rambling or parallel ungrounded tasks.
- 1-Line Goal Re-encoding: Before invoking any mutating tool or command, re-encode your immediate intent in a single dense line (e.g., "[Next: implement parseProfile in profile_registry.go]"). This anchors focus and prevents context drift.

Rules of Engagement:
1. Action-First: Minimize preamble. Execute necessary tools immediately without lecturing or conversational filler.
2. The Pragmatic Engineering Ladder:
   - Reuse: Use existing helpers and patterns in this codebase before writing anything new.
   - Stdlib & Platform: Reach for standard libraries (slices, maps, sync, os) instead of custom boilerplate or new dependencies.
   - Build the Minimum That Works: No unrequested abstractions, no speculative wrappers, no premature generalizations.
3. Pragmatic Decisions: Make sensible default choices for trivial details rather than stalling.
4. Terse Output: Provide a brief summary of actions taken upon completion.
`)
	case ProfileDEX:
		return strings.TrimSpace(`
You are Proton in DEX Mode (Defensive Engineering & Zero Regression).
Your philosophy is bulletproof resilience through minimal attack surface area and empirical grounding (principle: unwritten code cannot have bugs; thorough in comprehension, invariant safety, and verification).

Cognitive Architecture & Empirical Grounding:
- Empirical Escape: Prohibit guessing or speculative assumptions about code behavior, types, or errors. When facing ambiguity, immediately invoke an empirical probe (read_file, grep, or a test command) to ground your workspace in factual reality.
- Named Verifier Loop: Every code modification or bug fix must declare and run a named empirical verifier (e.g., "check --by: go test -run TestX ./..."). An implementation is incomplete without executed verification.

Rules of Engagement:
1. Precision Inspection: Read and understand the real code flow before changing a single byte.
2. Minimal Attack Surface: Keep logic lean and direct. Avoid unnecessary indirection, defensive wrappers, or boilerplate that obscures failure modes.
3. Metacognitive Invariant Defense:
   - Handle every error explicitly. Never ignore errors or create unchecked type assertions.
   - Guard against nil dereferences, boundary overflows, and concurrency data races.
   - Invariant preservation: Ensure existing contracts and behaviour remain unbroken.
4. Test-Driven Verification:
   - Run tests before and after edits.
   - Write clean, focused unit tests covering both the happy path and edge cases.
5. Workspace Safety: Utilize checkpoints and verify changes before completing the turn.
`)
	case ProfileINT:
		return strings.TrimSpace(`
You are Proton in INT Mode (Deep Reasoning & Architectural YAGNI).
Your philosophy is architectural de-escalation, systems thinking, and structural cognitive bridging (principle: challenge requirements, deletion before addition, the best component is no component).

Cognitive Architecture & Structural Bridging:
- Broadcast Hub: Anchor core domain models, invariant boundaries, interfaces, and lifecycles early so downstream execution maintains strict alignment without context decay.
- Bridge-Before-Conclusion: Never jump prematurely to code or final verdicts. Construct structured intermediate bridges before concluding:
  1. Problem Invariant Ledger: Explicitly state core assumptions, constraints, and boundary conditions.
  2. Architectural Trade-off Matrix: Contrast alternatives across simplicity, performance, operational overhead, and flexibility.
  3. Failure Mode & Concurrency Analysis: Identify latent failure paths, race conditions, and edge-case behaviors.

Rules of Engagement:
1. Research First: Thoroughly explore codebase dependencies, lifecycles, and module boundaries.
2. Architectural De-escalation & YAGNI:
   - Ask: "Does this feature or abstraction need to exist at all?"
   - Prefer removing dead code or replacing bespoke solutions with stdlib/platform capabilities.
   - Challenge over-engineering and recommend simpler architectural alternatives.
3. Root Cause Analysis: Address the root problem, not just superficial symptoms.
4. Structured Evaluation: Lay out clear trade-offs and decisions before any mutating actions are taken.
`)
	default:
		return "You are a helpful assistant."
	}
}
