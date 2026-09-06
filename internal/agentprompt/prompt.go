package agentprompt

import (
	"fmt"
	"sort"
	"strings"
)

const Version = "2"

type Spec struct {
	Role                string
	Profile             string
	Provider            string
	ModelID             string
	Workspace           string
	ToolNames           []string
	MaxRounds           int
	MaxToolCalls        int
	ReasoningRequested  string
	ReasoningEffective  string
	ReasoningSource     string
	ReasoningClamped    bool
	TaskPlanEnabled     bool
	DelegationEnabled   bool
	MutationEnabled     bool
	Skills              string
	ProjectInstructions string
	ExtraInstructions   []string
}

func Render(spec Spec) string {
	sections := []string{
		identitySection(),
		executionSection(),
		toolSection(spec),
		workspaceSection(spec),
	}
	if spec.TaskPlanEnabled {
		sections = append(sections, taskSection())
	}
	if spec.DelegationEnabled {
		sections = append(sections, delegationSection(spec))
	}
	if spec.MutationEnabled {
		sections = append(sections, verificationSection())
	}
	if section := modelSection(spec); section != "" {
		sections = append(sections, section)
	}
	if section := runtimeSection(spec); section != "" {
		sections = append(sections, section)
	}
	if role := strings.TrimSpace(spec.Role); role != "" {
		sections = append(sections, "# Profile\n"+role)
	}
	if project := strings.TrimSpace(spec.ProjectInstructions); project != "" {
		sections = append(sections, "# Project Instructions\n"+project)
	}
	for _, extra := range spec.ExtraInstructions {
		if text := strings.TrimSpace(extra); text != "" {
			sections = append(sections, "# Additional Instructions\n"+text)
		}
	}
	if skills := strings.TrimSpace(spec.Skills); skills != "" {
		sections = append(sections, "# Skills\n"+skills)
	}
	return "<proton-system-prompt version=\"" + Version + "\">\n" + strings.Join(sections, "\n\n") + "\n</proton-system-prompt>"
}

func IsManaged(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "<proton-system-prompt ") ||
		strings.HasPrefix(trimmed, "You are Proton, an autonomous coding agent operating inside a real workspace.") ||
		strings.HasPrefix(trimmed, "You are an Explorer subagent in Proton.") ||
		strings.HasPrefix(trimmed, "You are a Code Reviewer subagent in Proton.") ||
		strings.HasPrefix(trimmed, "You are a Worker subagent in Proton.") ||
		strings.HasPrefix(trimmed, "You are Proton in POW Mode") ||
		strings.HasPrefix(trimmed, "You are Proton in DEX Mode") ||
		strings.HasPrefix(trimmed, "You are Proton in INT Mode")
}

func identitySection() string {
	return `# Identity
You are Proton, an autonomous coding agent operating in a real workspace.`
}

func executionSection() string {
	return `# Execution Contract
- Work from empirical repository state. Never guess workspace contents, repository state, test results, or external facts when a tool can establish them.
- Keep going until the requested task is resolved as far as the available tools and permissions allow.
- Fix root causes when practical; keep changes minimal, focused, and consistent with existing project conventions.
- For requested implementation, perform the edits instead of only describing them, then verify the result.
- Preserve unrelated user work. Never revert or overwrite pre-existing changes merely to make your task easier.
- Do not create commits, branches, releases, or deployments unless the user explicitly requested them.
- Communicate through assistant text, not shell output, generated files, or code comments.`
}

func toolSection(spec Spec) string {
	lines := []string{
		"# Tool Protocol",
		"- Use tools whenever the answer depends on the current workspace, repository state, files, commands, tests, or external facts.",
		"- Tool names are exact identifiers. Call only tools actually provided for this request. Never prefix, qualify, rename, or invent a tool name.",
		"- Treat tool errors as observations. Correct the call when possible instead of repeatedly issuing the same invalid request.",
		"- Planning/status tools do not count as evidence about source code or repository state.",
	}
	names := uniqueSorted(spec.ToolNames)
	if len(names) > 0 {
		lines = append(lines, "- Available tools: "+strings.Join(names, ", ")+".")
	}
	return strings.Join(lines, "\n")
}

func workspaceSection(spec Spec) string {
	lines := []string{"# Workspace"}
	if root := strings.TrimSpace(spec.Workspace); root != "" {
		lines = append(lines, "- Workspace root: "+root)
	}
	lines = append(lines,
		"- Inspect relevant code and nearby conventions before making claims about the existing implementation.",
		"- Read narrowly first, then broaden search only when needed. Prefer targeted repository tools over speculative prose.",
	)
	return strings.Join(lines, "\n")
}

func taskSection() string {
	return `# Task Plan Protocol
- The task plan is parent-owned coordination metadata, not evidence about code.
- Call get_todo before changing task state so you have the latest revision.
- Preserve stable task IDs and existing tasks unless deletion is intentional.
- Mark a task completed only after its work is actually complete; agent process completion alone is not proof of task completion.
- If update_todo reports a revision conflict, refresh with get_todo and retry from the new snapshot.`
}

func delegationSection(spec Spec) string {
	text := `# Delegation Protocol
- Use delegate_task when a bounded parallel investigation or implementation will reduce parent context or shorten the critical path.
- For broad multi-file exploration, prefer explorer or reviewer subagents when available; keep trivial targeted lookups in the parent.
- Use exact lifecycle tools (wait_agent, get_agent, list_agents, cancel_agent) only when they are available.
- A child summary is evidence to inspect, not automatic verification of parent task completion.`
	if strings.Contains(strings.ToLower(spec.ModelID), "gemini") {
		text += "\n- Gemini guidance: explicitly use delegate_task for broad exploration instead of silently doing all discovery in prose or treating get_todo as workspace grounding."
	}
	return text
}

func verificationSection() string {
	return `# Editing And Verification
- Before editing, understand the current file state and preserve unrelated changes.
- After the final mutation, run an appropriate empirical verifier such as tests, build, lint, typecheck, or git diff --check before declaring the change verified.
- A verifier run before the final mutation does not verify later changes.
- Report verification honestly; do not claim tests passed unless they actually ran successfully.`
}

func modelSection(spec Spec) string {
	id := strings.ToLower(strings.TrimSpace(spec.ModelID))
	switch {
	case strings.Contains(id, "gemini"):
		return `# Model Guidance
- Prefer explicit tool calls over unsupported assumptions when repository facts are needed.
- When a specialized tool exists for an action, use that tool instead of describing the action in prose.
- Do not invent tool namespaces or prefixes.`
	case strings.Contains(id, "grok"):
		return `# Model Guidance
- Verify tool-dependent claims with the relevant tool result before presenting them as facts.
- Use only capabilities that are actually exposed in this request.`
	case strings.Contains(id, "codex"):
		return `# Model Guidance
- Continue through inspection, implementation, and verification without stopping at a plan when the user requested execution.`
	default:
		return ""
	}
}

func runtimeSection(spec Spec) string {
	var lines []string
	if provider := strings.TrimSpace(spec.Provider); provider != "" {
		lines = append(lines, "provider="+provider)
	}
	if modelID := strings.TrimSpace(spec.ModelID); modelID != "" {
		lines = append(lines, "model="+modelID)
	}
	if profile := strings.TrimSpace(spec.Profile); profile != "" {
		lines = append(lines, "profile="+profile)
	}
	if spec.MaxRounds > 0 {
		lines = append(lines, fmt.Sprintf("max_rounds=%d", spec.MaxRounds))
	}
	if spec.MaxToolCalls > 0 {
		lines = append(lines, fmt.Sprintf("max_tool_calls=%d", spec.MaxToolCalls))
	}
	if requested := strings.TrimSpace(spec.ReasoningRequested); requested != "" {
		lines = append(lines, "reasoning_requested="+requested)
	}
	if effective := strings.TrimSpace(spec.ReasoningEffective); effective != "" {
		lines = append(lines, "reasoning_effective="+effective)
	}
	if source := strings.TrimSpace(spec.ReasoningSource); source != "" {
		lines = append(lines, "reasoning_source="+source)
	}
	if spec.ReasoningClamped {
		lines = append(lines, "reasoning_clamped=true")
	}
	if len(lines) == 0 {
		return ""
	}
	return "# Runtime Context\n" + strings.Join(lines, "\n")
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
