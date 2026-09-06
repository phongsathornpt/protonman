package tui

import (
	"encoding/json"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
)

var titleRegex = regexp.MustCompile(`(?i)<title[^>]*>([\s\S]*?)</title>`)

// extractToolTarget inspects tool arguments and returns a human-facing target
// string (e.g. URL, filepath, pattern, command) and the normalized tool.Kind.
func extractToolTarget(name string, kind tool.Kind, args json.RawMessage) (string, tool.Kind) {
	if kind == "" {
		kind = guessToolKind(name)
	}

	var values map[string]any
	if len(args) > 0 {
		_ = json.Unmarshal(args, &values)
	}

	switch kind {
	case tool.KindWebFetch:
		if urlStr, ok := values["url"].(string); ok && strings.TrimSpace(urlStr) != "" {
			return strings.TrimSpace(urlStr), kind
		}
	case tool.KindWebSearch:
		if query, ok := values["query"].(string); ok && strings.TrimSpace(query) != "" {
			return fmt.Sprintf("%q", strings.TrimSpace(query)), kind
		}
	case tool.KindRead:
		if name == "list_dir" {
			for _, key := range []string{"path", "dir_path", "directory"} {
				if path, ok := values[key].(string); ok && strings.TrimSpace(path) != "" {
					return strings.TrimSpace(path), kind
				}
			}
			return ".", kind
		}
		if name == "git_status" {
			if path, ok := values["path"].(string); ok && strings.TrimSpace(path) != "" {
				return strings.TrimSpace(path), kind
			}
			return "", kind
		}
		if path, ok := values["path"].(string); ok && strings.TrimSpace(path) != "" {
			return strings.TrimSpace(path), kind
		}
	case tool.KindGrep:
		pattern, _ := values["pattern"].(string)
		path, _ := values["path"].(string)
		pattern = strings.TrimSpace(pattern)
		path = strings.TrimSpace(path)
		if pattern != "" && path != "" && path != "." {
			return fmt.Sprintf("%q in %s", pattern, path), kind
		}
		if pattern != "" {
			return fmt.Sprintf("%q", pattern), kind
		}
	case tool.KindTask:
		if items, ok := values["items"].([]any); ok {
			return fmt.Sprintf("%d tasks", len(items)), kind
		}
		return "task plan", kind
	case tool.KindAgent:
		if name == "delegate_task" {
			task, _ := values["task"].(string)
			profile, _ := values["profile"].(string)
			if strings.TrimSpace(profile) != "" {
				return fmt.Sprintf("[%s] %s", strings.TrimSpace(profile), truncateWithEllipsis(strings.TrimSpace(task), 40)), kind
			}
		}
		if id, ok := values["agent_id"].(string); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id), kind
		}
		return "subagents", kind
	case tool.KindBash:
		if cmd, ok := values["command"].(string); ok && strings.TrimSpace(cmd) != "" {
			return strings.TrimSpace(cmd), kind
		}
	case tool.KindEdit:
		for _, key := range []string{"path", "file", "filename", "target"} {
			if path, ok := values[key].(string); ok && strings.TrimSpace(path) != "" {
				return strings.TrimSpace(path), kind
			}
		}
	}

	if name == "activate_skill" {
		if skillName, ok := values["name"].(string); ok && strings.TrimSpace(skillName) != "" {
			return fmt.Sprintf("%q", strings.TrimSpace(skillName)), kind
		}
	}

	if name == "delegate_task" {
		task, _ := values["task"].(string)
		profile, _ := values["profile"].(string)
		task = strings.TrimSpace(task)
		profile = strings.TrimSpace(profile)
		if profile != "" && task != "" {
			return fmt.Sprintf("[%s] %s", profile, truncateWithEllipsis(task, 40)), kind
		}
		if task != "" {
			return truncateWithEllipsis(task, 40), kind
		}
	}

	if name == "checkpoint_restore" {
		if id, ok := values["checkpoint_id"].(string); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id), kind
		}
	}

	// Heuristic fallback for arbitrary MCP and custom tools:
	for _, key := range []string{"url", "path", "file", "query", "pattern", "command", "target", "task", "name"} {
		if val, ok := values[key].(string); ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val), kind
		}
	}

	return "", kind
}

func guessToolKind(name string) tool.Kind {
	switch name {
	case "web_fetch":
		return tool.KindWebFetch
	case "web_search":
		return tool.KindWebSearch
	case "read_file", "list_dir", "git_status":
		return tool.KindRead
	case "grep":
		return tool.KindGrep
	case "bash":
		return tool.KindBash
	case "write_file", "search_replace", "apply_patch":
		return tool.KindEdit
	case "get_todo", "update_todo":
		return tool.KindTask
	case "delegate_task", "wait_agent", "get_agent", "list_agents", "cancel_agent":
		return tool.KindAgent
	default:
		return ""
	}
}

// toolKindGlyph returns the appropriate category glyph for a tool.
func toolKindGlyph(kind tool.Kind, name string) string {
	switch kind {
	case tool.KindWebFetch, tool.KindWebSearch:
		return glyphWeb
	case tool.KindRead:
		if name == "list_dir" {
			return glyphDir
		}
		if name == "git_status" {
			return "⌥ "
		}
		return glyphRead
	case tool.KindGrep:
		return glyphSearch
	case tool.KindBash:
		return glyphExec
	case tool.KindEdit:
		return glyphEdit
	case tool.KindTask:
		return glyphTodoActive
	case tool.KindAgent:
		return glyphAgent
	}

	switch name {
	case "activate_skill":
		return glyphSkill
	case "delegate_task":
		return glyphAgent
	default:
		return glyphGeneric
	}
}

// summarizeToolOutput produces a concise, high-signal semantic analysis summary
// from a tool's output body rather than dumping raw payloads into the TUI.
func summarizeToolOutput(name string, kind tool.Kind, target string, body string, exitCode *int, truncated bool) string {
	bodyTrimmed := strings.TrimSpace(body)

	switch kind {
	case tool.KindWebFetch:
		return summarizeWebFetch(bodyTrimmed, truncated)
	case tool.KindWebSearch:
		return summarizeWebSearch(bodyTrimmed)
	case tool.KindRead:
		if name == "list_dir" {
			return summarizeListDir(bodyTrimmed)
		}
		if name == "git_status" {
			return summarizeGitStatus(bodyTrimmed)
		}
		return summarizeReadFileTarget(target, bodyTrimmed, truncated)
	case tool.KindGrep:
		return summarizeGrep(bodyTrimmed, truncated)
	case tool.KindEdit:
		return summarizeEdit(name, bodyTrimmed)
	case tool.KindTask:
		if name == "get_todo" {
			return summarizeTodoSnapshot(bodyTrimmed)
		}
		return summarizeTodoUpdate(bodyTrimmed)
	case tool.KindAgent:
		return summarizeAgentTool(name, bodyTrimmed)
	case tool.KindBash:
		if exitCode != nil {
			return fmt.Sprintf("exit %d", *exitCode)
		}
	}

	if name == "activate_skill" {
		if skillName := extractSkillContentName(body); skillName != "" {
			return fmt.Sprintf("Activated skill %q", skillName)
		}
		return "activated"
	}

	if name == "checkpoint_restore" {
		return "restored checkpoint"
	}

	// Fallback for MCP or generic tools
	if bodyTrimmed == "" {
		return "completed"
	}
	lines := strings.Count(bodyTrimmed, "\n") + 1
	if lines > 1 {
		return fmt.Sprintf("%d lines (%s)", lines, formatByteSize(len(bodyTrimmed)))
	}
	return formatByteSize(len(bodyTrimmed))
}

func summarizeAgentTool(name, body string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil {
		return "agent updated"
	}
	if name == "list_agents" {
		agents, _ := payload["agents"].([]any)
		active := 0
		for _, raw := range agents {
			if m, ok := raw.(map[string]any); ok {
				if st, _ := m["state"].(string); st == "queued" || st == "running" || st == "canceling" {
					active++
				}
			}
		}
		return fmt.Sprintf("%d agents · %d active", len(agents), active)
	}
	id, _ := payload["agent_id"].(string)
	status, _ := payload["status"].(string)
	if agentObj, ok := payload["agent"].(map[string]any); ok {
		if id == "" {
			id, _ = agentObj["id"].(string)
		}
		if status == "" {
			status, _ = agentObj["state"].(string)
		}
	}
	resultSummary := agentResultSummary(payload)
	switch name {
	case "delegate_task":
		if id != "" {
			return fmt.Sprintf("spawned %s · %s", id, status)
		}
		return "subagent spawned"
	case "wait_agent":
		if status == "queued" || status == "running" || status == "canceling" {
			return fmt.Sprintf("waiting for %s · %s", id, status)
		}
		return joinAgentCompletionSummary(id, status, resultSummary)
	case "get_agent":
		return joinAgentCompletionSummary(id, status, resultSummary)
	case "cancel_agent":
		return fmt.Sprintf("cancel requested · %s", id)
	default:
		if id != "" {
			return fmt.Sprintf("%s · %s", id, status)
		}
	}
	return "agent updated"
}

func agentResultSummary(payload map[string]any) string {
	result, _ := payload["result"].(map[string]any)
	if result == nil {
		return ""
	}
	summary, _ := result["summary"].(string)
	return truncateWithEllipsis(strings.TrimSpace(summary), 96)
}

func joinAgentCompletionSummary(id, status, summary string) string {
	base := strings.TrimSpace(strings.Join([]string{id, status}, " · "))
	if summary == "" {
		return base
	}
	if base == "" {
		return summary
	}
	return base + " · " + summary
}

func summarizeTodoSnapshot(body string) string {
	var payload struct {
		Revision uint64            `json:"revision"`
		Items    []json.RawMessage `json:"items"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return "task snapshot"
	}
	return fmt.Sprintf("Tasks %d · revision %d", len(payload.Items), payload.Revision)
}

func summarizeTodoUpdate(body string) string {
	var payload struct {
		Total      int `json:"total"`
		Completed  int `json:"completed"`
		InProgress int `json:"in_progress"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return "tasks updated"
	}
	summary := fmt.Sprintf("Tasks updated · %d/%d complete", payload.Completed, payload.Total)
	if payload.InProgress > 0 {
		summary += fmt.Sprintf(" · %d active", payload.InProgress)
	}
	return summary
}

func summarizeWebFetch(body string, truncated bool) string {
	if body == "" {
		return "0 B"
	}
	if strings.HasPrefix(body, "[binary content omitted") {
		return body
	}

	sizeStr := formatByteSize(len(body))
	if truncated {
		sizeStr += "+ truncated"
	}

	// Try extracting HTML title
	if matches := titleRegex.FindStringSubmatch(body); len(matches) > 1 {
		rawTitle := html.UnescapeString(strings.TrimSpace(matches[1]))
		rawTitle = strings.Join(strings.Fields(rawTitle), " ")
		if rawTitle != "" {
			return fmt.Sprintf("%q (%s)", truncateWithEllipsis(rawTitle, 45), sizeStr)
		}
	}

	// Check if JSON
	if (strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}")) ||
		(strings.HasPrefix(body, "[") && strings.HasSuffix(body, "]")) {
		var anyVal any
		if json.Unmarshal([]byte(body), &anyVal) == nil {
			switch val := anyVal.(type) {
			case []any:
				return fmt.Sprintf("JSON array (%d items, %s)", len(val), sizeStr)
			case map[string]any:
				return fmt.Sprintf("JSON object (%d keys, %s)", len(val), sizeStr)
			}
		}
	}

	lines := strings.Count(body, "\n") + 1
	return fmt.Sprintf("%d lines (%s)", lines, sizeStr)
}

func summarizeWebSearch(body string) string {
	if body == "" {
		return "0 results"
	}
	lines := strings.Split(body, "\n")
	nonEmpty := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
	}
	if nonEmpty == 1 {
		return "1 result found"
	}
	return fmt.Sprintf("%d results found", nonEmpty)
}

func summarizeReadFile(body string, truncated bool) string {
	return summarizeReadFileTarget("", body, truncated)
}

func summarizeReadFileTarget(target string, body string, truncated bool) string {
	if body == "" {
		return "0 B (empty file)"
	}
	sizeStr := formatByteSize(len(body))
	if truncated {
		sizeStr += "+ truncated"
	}
	lines := strings.Count(body, "\n") + 1
	var lineStr string
	if lines == 1 {
		lineStr = fmt.Sprintf("1 line (%s)", sizeStr)
	} else {
		lineStr = fmt.Sprintf("%d lines (%s)", lines, sizeStr)
	}
	if ft := detectFileType(target); ft != "" {
		return fmt.Sprintf("%s · %s", lineStr, ft)
	}
	return lineStr
}

// detectFileType returns a concise human-readable language or file format label.
func detectFileType(path string) string {
	clean := strings.ToLower(strings.TrimSpace(path))
	if clean == "" {
		return ""
	}
	clean = strings.Trim(clean, `"'`)
	ext := filepath.Ext(clean)
	switch ext {
	case ".go":
		return "Go"
	case ".json":
		return "JSON"
	case ".toml":
		return "TOML"
	case ".yaml", ".yml":
		return "YAML"
	case ".md", ".markdown":
		return "Markdown"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".sh", ".bash", ".zsh":
		return "Shell"
	case ".sql":
		return "SQL"
	case ".html", ".htm":
		return "HTML"
	case ".css":
		return "CSS"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "C++"
	case ".mod":
		return "Go Module"
	case ".sum":
		return "Go Sum"
	case ".proto":
		return "Protobuf"
	default:
		base := filepath.Base(clean)
		if base == "dockerfile" {
			return "Dockerfile"
		}
		if base == "makefile" {
			return "Makefile"
		}
		return ""
	}
}

// formatPathSegmentsStyled visually differentiates directory path from base filename.
func formatPathSegmentsStyled(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	// For grep query with " in ":
	if strings.Contains(target, " in ") {
		parts := strings.SplitN(target, " in ", 2)
		return toolTargetStyle.Render(parts[0]) + mutedStyle.Render(" in ") + formatPathSegmentsStyled(parts[1])
	}
	// For quoted strings (e.g. web search query):
	if strings.HasPrefix(target, `"`) && strings.HasSuffix(target, `"`) {
		return toolTargetStyle.Render(target)
	}
	// Check if URL:
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return toolTargetStyle.Render(target)
	}

	idx := strings.LastIndex(target, "/")
	if idx == -1 {
		idx = strings.LastIndex(target, `\`)
	}
	if idx == -1 {
		return toolTargetStyle.Render(target)
	}

	dir := target[:idx+1]
	base := target[idx+1:]
	return toolDirStyle.Render(dir) + toolTargetStyle.Render(base)
}

// extractReadFileExcerpt extracts a 1-line structural header preview (e.g. package, shebang, title).
func extractReadFileExcerpt(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		// Structural headers only (avoid printing arbitrary code statements)
		if strings.HasPrefix(trimmed, "package ") ||
			strings.HasPrefix(trimmed, "#!/") ||
			strings.HasPrefix(trimmed, "# ") ||
			strings.HasPrefix(trimmed, "## ") ||
			strings.HasPrefix(trimmed, "module ") {
			return truncateWithEllipsis(trimmed, 65)
		}
	}
	return ""
}

func summarizeListDir(body string) string {
	if body == "" {
		return "empty directory"
	}
	lines := strings.Split(body, "\n")
	dirs, files := 0, 0
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "dir ") {
			dirs++
		} else {
			files++
		}
	}
	total := dirs + files
	if total == 0 {
		return "empty directory"
	}
	return fmt.Sprintf("%d items (%d dirs, %d files)", total, dirs, files)
}

func summarizeGrep(body string, truncated bool) string {
	if body == "" {
		return "no matches found"
	}
	lines := strings.Split(body, "\n")
	matchCount := 0
	files := make(map[string]struct{})
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		matchCount++
		if idx := strings.Index(trimmed, ":"); idx != -1 {
			files[trimmed[:idx]] = struct{}{}
		}
	}
	if matchCount == 0 {
		return "no matches found"
	}
	fileCount := len(files)
	matchWord := "matches"
	if matchCount == 1 {
		matchWord = "match"
	}
	fileWord := "files"
	if fileCount == 1 {
		fileWord = "file"
	}
	suffix := ""
	if truncated {
		suffix = " (truncated)"
	}
	return fmt.Sprintf("%d %s across %d %s%s", matchCount, matchWord, fileCount, fileWord, suffix)
}

func summarizeGitStatus(body string) string {
	if body == "" || strings.Contains(body, "nothing to commit") || strings.Contains(body, "working tree clean") {
		return "working tree clean"
	}
	lines := strings.Split(body, "\n")
	modified, untracked, staged := 0, 0, 0
	for _, l := range lines {
		if len(l) < 3 {
			continue
		}
		x := l[0]
		y := l[1]
		if x == '?' && y == '?' {
			untracked++
			continue
		}
		if x == 'M' || x == 'A' || x == 'D' || x == 'R' || x == 'C' {
			staged++
		}
		if y == 'M' || y == 'D' {
			modified++
		}
	}
	parts := make([]string, 0, 3)
	if staged > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", staged))
	}
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", modified))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}
	if len(parts) == 0 {
		return "status updated"
	}
	return strings.Join(parts, ", ")
}

func summarizeEdit(name string, body string) string {
	if body == "" {
		return "updated"
	}
	switch name {
	case "write_file":
		lines := strings.Count(body, "\n") + 1
		return fmt.Sprintf("%d lines written (%s)", lines, formatByteSize(len(body)))
	case "search_replace":
		return "1 replacement applied"
	case "apply_patch":
		return "patch applied"
	default:
		return "file updated"
	}
}

// shouldSuppressBody returns true if raw tool body dumping should be suppressed
// in the primary conversation viewport because the semantic header already summarizes it.
func shouldSuppressBody(kind tool.Kind, name string) bool {
	switch kind {
	case tool.KindWebFetch, tool.KindWebSearch, tool.KindRead, tool.KindAgent:
		return true
	}
	switch name {
	case "activate_skill", "delegate_task", "checkpoint_restore":
		return true
	default:
		return false
	}
}

// formatOutputFold folds long text outputs with a fold hint.
func formatOutputFold(lines []string, maxVisible int) []string {
	if len(lines) <= maxVisible {
		return lines
	}
	if maxVisible < 2 {
		maxVisible = 2
	}
	headCount := maxVisible - 1
	hidden := len(lines) - maxVisible
	out := make([]string, 0, maxVisible+1)
	out = append(out, lines[:headCount]...)
	foldMsg := toolFoldStyle.Render(fmt.Sprintf("… (%d lines hidden · ctrl+t for full output)", hidden))
	out = append(out, foldMsg)
	out = append(out, lines[len(lines)-1])
	return out
}

func formatByteSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024.0)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024.0*1024.0))
}
