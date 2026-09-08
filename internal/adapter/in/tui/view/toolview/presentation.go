package toolview

import (
	"encoding/json"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

var titleRegex = regexp.MustCompile(`(?i)<title[^>]*>([\s\S]*?)</title>`)

// ExtractTarget inspects tool arguments and returns a human-facing target
// string (e.g. URL, filepath, pattern, command) and the normalized tool.Kind.
func ExtractTarget(name string, kind tool.Kind, args json.RawMessage) (string, tool.Kind) {
	call := tool.Call{Name: name, Arguments: args}
	target := call.Target()
	if kind == "" {
		kind = tool.KindForName(name)
	}
	return target, kind
}

func IsAgentLifecycleTool(name string) bool {
	switch name {
	case "wait_agent", "get_agent", "list_agents", "cancel_agent", "resume_agent":
		return true
	default:
		return false
	}
}

// KindGlyph returns the appropriate category glyph for a tool.
func KindGlyph(kind tool.Kind, name string) string {
	switch kind {
	case tool.KindWebFetch, tool.KindWebSearch:
		return tuistyle.GlyphWeb
	case tool.KindRead:
		if tool.CanonicalName(name) == "ls" {
			return tuistyle.GlyphDir
		}
		return tuistyle.GlyphRead
	case tool.KindGrep:
		return tuistyle.GlyphSearch
	case tool.KindGit:
		return "⌥ "
	case tool.KindBash:
		return tuistyle.GlyphExec
	case tool.KindEdit:
		return tuistyle.GlyphEdit
	case tool.KindTask:
		return tuistyle.GlyphTodoActive
	case tool.KindAgent:
		return tuistyle.GlyphAgent
	}

	switch name {
	case "activate_skill":
		return tuistyle.GlyphSkill
	case "delegate_task":
		return tuistyle.GlyphAgent
	default:
		return tuistyle.GlyphGeneric
	}
}

// SummarizeOutput produces a concise, high-signal semantic analysis summary
// from a tool's output body rather than dumping raw payloads into the TUI.
// ExtractSkillContentName extracts the activated skill name from structured skill output.
func ExtractSkillContentName(body string) string {
	for _, quote := range []string{`name="`, `name='`} {
		idx := strings.Index(body, quote)
		if idx == -1 {
			continue
		}
		rest := body[idx+len(quote):]
		if end := strings.IndexAny(rest, `"'`); end != -1 {
			return rest[:end]
		}
	}
	return ""
}

func SummarizeOutput(name string, kind tool.Kind, target string, body string, exitCode *int, truncated bool) string {
	bodyTrimmed := strings.TrimSpace(body)

	switch kind {
	case tool.KindWebFetch:
		return summarizeWebFetch(bodyTrimmed, truncated)
	case tool.KindWebSearch:
		return summarizeWebSearch(bodyTrimmed)
	case tool.KindRead:
		if tool.CanonicalName(name) == "ls" {
			return summarizeListDir(bodyTrimmed, truncated)
		}
		return summarizeReadFileTarget(target, bodyTrimmed, truncated)
	case tool.KindGrep:
		return summarizeGrep(bodyTrimmed, truncated)
	case tool.KindGit:
		return summarizeGitStatus(bodyTrimmed)
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
		if skillName := ExtractSkillContentName(body); skillName != "" {
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
		if timedOut, _ := payload["timed_out"].(bool); timedOut {
			return "no new agent activity"
		}
		events, _ := payload["events"].([]any)
		if len(events) > 1 {
			return fmt.Sprintf("%d agent lifecycle events", len(events))
		}
		if event, ok := payload["event"].(map[string]any); ok {
			eventID, _ := event["agent_id"].(string)
			eventKind, _ := event["kind"].(string)
			if eventID != "" || eventKind != "" {
				return strings.Trim(strings.Join([]string{eventID, eventKind}, " · "), " ·")
			}
		}
		return "agent activity received"
	case "get_agent":
		return joinAgentCompletionSummary(id, status, resultSummary)
	case "cancel_agent":
		return fmt.Sprintf("cancel requested · %s", id)
	case "resume_agent":
		from, _ := payload["resumed_from"].(string)
		if from != "" && id != "" {
			return fmt.Sprintf("resumed %s as %s · %s", from, id, status)
		}
		return joinAgentCompletionSummary(id, status, resultSummary)
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
	return textview.TruncateEllipsis(strings.TrimSpace(summary), 96)
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
		Changes    struct {
			Added     int `json:"added"`
			Removed   int `json:"removed"`
			Started   int `json:"started"`
			Completed int `json:"completed"`
			Reopened  int `json:"reopened"`
		} `json:"changes"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return "tasks updated"
	}
	parts := []string{"Tasks updated"}
	if payload.Changes.Completed > 0 {
		parts = append(parts, fmt.Sprintf("%d completed", payload.Changes.Completed))
	}
	if payload.Changes.Started > 0 {
		parts = append(parts, fmt.Sprintf("%d started", payload.Changes.Started))
	}
	if payload.Changes.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", payload.Changes.Added))
	}
	if payload.Changes.Removed > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", payload.Changes.Removed))
	}
	if payload.Changes.Reopened > 0 {
		parts = append(parts, fmt.Sprintf("%d reopened", payload.Changes.Reopened))
	}
	if len(parts) > 1 {
		return strings.Join(parts, " · ")
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
			return fmt.Sprintf("%q (%s)", textview.TruncateEllipsis(rawTitle, 45), sizeStr)
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
	if strings.HasPrefix(body, "image ") || strings.HasPrefix(body, "structured ") || strings.HasPrefix(body, "binary ") {
		return textview.TruncateEllipsis(body, 120)
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

// FormatPath visually differentiates directory path from base filename.
func FormatPath(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	// For grep query with " in ":
	if strings.Contains(target, " in ") {
		parts := strings.SplitN(target, " in ", 2)
		pattern := strings.Trim(parts[0], `"`)
		if ansi.StringWidth(pattern) > 30 {
			pattern = textview.TruncateEllipsis(pattern, 28)
		}
		return tuistyle.ToolTargetStyle.Render(fmt.Sprintf("%q", pattern)) + tuistyle.MutedStyle.Render(" in ") + FormatPath(parts[1])
	}
	// For quoted strings (e.g. web search query or grep pattern):
	if strings.HasPrefix(target, `"`) && strings.HasSuffix(target, `"`) {
		inner := strings.Trim(target, `"`)
		if ansi.StringWidth(inner) > 36 {
			inner = textview.TruncateEllipsis(inner, 34)
			return tuistyle.ToolTargetStyle.Render(fmt.Sprintf("%q", inner))
		}
		return tuistyle.ToolTargetStyle.Render(target)
	}
	// Check if URL:
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if ansi.StringWidth(target) > 50 {
			target = textview.TruncateEllipsis(target, 48)
		}
		return tuistyle.ToolTargetStyle.Render(target)
	}

	idx := strings.LastIndex(target, "/")
	if idx == -1 {
		idx = strings.LastIndex(target, `\`)
	}
	if idx == -1 {
		return tuistyle.ToolTargetStyle.Render(target)
	}

	dir := target[:idx+1]
	base := target[idx+1:]
	return tuistyle.ToolDirStyle.Render(dir) + tuistyle.ToolTargetStyle.Render(base)
}

// extractReadFileExcerpt extracts a 1-line structural header preview (e.g. package, shebang, title).
func ExtractReadFileExcerpt(body string) string {
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
			return textview.TruncateEllipsis(trimmed, 65)
		}
	}
	return ""
}

func summarizeListDir(body string, truncated bool) string {
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
	suffix := ""
	if truncated {
		suffix = " + truncated"
	}
	return fmt.Sprintf("%d items (%d dirs, %d files)%s", total, dirs, files, suffix)
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
	if tool.CanonicalName(name) == "edit" && name == "edit" {
		lower := strings.ToLower(body)
		switch {
		case strings.Contains(lower, "restored checkpoint"):
			return "restored checkpoint"
		case strings.Contains(lower, "wrote file successfully"):
			return "saved"
		case strings.Contains(lower, "success. updated"), strings.Contains(lower, "success. added"), strings.Contains(lower, "success. deleted"):
			return "patch applied"
		case strings.Contains(lower, "has been updated"), strings.Contains(lower, "has been created"):
			return "1 replacement applied"
		}
	}
	switch name {
	case "write_file":
		return "saved"
	case "search_replace":
		return "1 replacement applied"
	case "apply_patch":
		return "patch applied"
	case "checkpoint_restore":
		return "restored checkpoint"
	default:
		return "file updated"
	}
}

// ShouldSuppressBody returns true if raw tool body dumping should be suppressed
// in the primary conversation viewport because the semantic header already summarizes it.
func ShouldSuppressBody(kind tool.Kind, name string) bool {
	switch kind {
	case tool.KindWebFetch, tool.KindWebSearch, tool.KindRead, tool.KindGit, tool.KindAgent, tool.KindTask, tool.KindEdit:
		return true
	}
	switch name {
	case "activate_skill", "delegate_task", "checkpoint_restore", "get_todo", "update_todo":
		return true
	default:
		return false
	}
}

// FormatOutputFold folds long text outputs with a fold hint.
func FormatOutputFold(lines []string, maxVisible int) []string {
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
	lineWord := "lines"
	if hidden == 1 {
		lineWord = "line"
	}
	foldMsg := tuistyle.ToolFoldStyle.Render(fmt.Sprintf("… (%d %s hidden · ctrl+t for full output)", hidden, lineWord))
	out = append(out, foldMsg)
	out = append(out, lines[len(lines)-1])
	return out
}

// StyleDiffLine checks if a line looks like a diff line and applies syntax coloring.
func StyleDiffLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "+++ ") || strings.HasPrefix(trimmed, "--- ") {
		return tuistyle.MutedStyle.Bold(true).Render(line), true
	}
	if strings.HasPrefix(trimmed, "+") {
		return tuistyle.DiffAddStyle.Render(line), true
	}
	if strings.HasPrefix(trimmed, "-") {
		return tuistyle.DiffDeleteStyle.Render(line), true
	}
	if strings.HasPrefix(trimmed, "@@") {
		return tuistyle.DiffHunkStyle.Render(line), true
	}
	if strings.HasPrefix(trimmed, "diff --git ") || strings.HasPrefix(trimmed, "index ") {
		return tuistyle.MutedStyle.Bold(true).Render(line), true
	}
	return line, false
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

// FormatGrepView formats grep lines with syntax styling, keyword highlighting,
// and horizontal width clamping to prevent wrapping explosion in the viewport.
func FormatGrepView(lines []string, target string, width int) []string {
	if len(lines) == 0 {
		return nil
	}

	terms := extractGrepQueryTerms(target)
	formatted := make([]string, 0, len(lines))
	contentWidth := max(20, width-6)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			formatted = append(formatted, tuistyle.ToolFoldStyle.Render(trimmed))
			continue
		}
		parts := strings.SplitN(trimmed, ":", 3)
		if len(parts) == 3 {
			file := parts[0]
			lineNum := parts[1]
			content := parts[2]

			avail := contentWidth - ansi.StringWidth(file) - len(lineNum) - 4
			cleanContent := strings.TrimSpace(content)
			if avail > 10 && ansi.StringWidth(cleanContent) > avail {
				cleanContent = textview.TruncateEllipsis(cleanContent, avail)
			}

			highlightedContent := highlightGrepTerms(cleanContent, terms)
			lineStr := tuistyle.ToolTargetStyle.Render(file) + tuistyle.MutedStyle.Render(":"+lineNum+": ") + highlightedContent
			formatted = append(formatted, lineStr)
		} else {
			if ansi.StringWidth(trimmed) > contentWidth {
				trimmed = textview.TruncateEllipsis(trimmed, contentWidth)
			}
			formatted = append(formatted, tuistyle.BodyStyle.Render(trimmed))
		}
	}

	return FormatOutputFold(formatted, 4)
}

func extractGrepQueryTerms(target string) []string {
	clean := strings.TrimSpace(target)
	if strings.Contains(clean, " in ") {
		clean = strings.SplitN(clean, " in ", 2)[0]
	}
	clean = strings.Trim(clean, `"`)
	if clean == "" {
		return nil
	}
	rawTerms := strings.Split(clean, "|")
	terms := make([]string, 0, len(rawTerms))
	for _, t := range rawTerms {
		t = strings.TrimSpace(t)
		if len(t) >= 2 {
			terms = append(terms, t)
		}
	}
	return terms
}

func highlightGrepTerms(content string, terms []string) string {
	if len(terms) == 0 || content == "" {
		return tuistyle.BodyStyle.Render(content)
	}
	result := content
	for _, term := range terms {
		idx := strings.Index(strings.ToLower(result), strings.ToLower(term))
		if idx != -1 && idx+len(term) <= len(result) {
			matched := result[idx : idx+len(term)]
			return tuistyle.BodyStyle.Render(result[:idx]) + tuistyle.BrandStyle.Render(matched) + tuistyle.BodyStyle.Render(result[idx+len(term):])
		}
	}
	return tuistyle.BodyStyle.Render(result)
}
