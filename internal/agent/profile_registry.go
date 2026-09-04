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

	case ProfileWorker:
		// Worker has full coding tools but cannot delegate
		return true

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
	default:
		return "You are a helpful assistant."
	}
}
