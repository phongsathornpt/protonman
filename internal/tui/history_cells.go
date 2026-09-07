package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/projectTHORN/proton/internal/tool"
)

// HistoryCellKind identifies the semantic role of one transcript cell.
type HistoryCellKind uint8

const (
	// HistoryCellUnknown is the invalid zero value.
	HistoryCellUnknown HistoryCellKind = iota
	HistoryCellUser
	HistoryCellAssistant
	HistoryCellTool
	HistoryCellSystem
	HistoryCellError
)

// String returns the human-readable spelling of a history cell kind.
func (k HistoryCellKind) String() string {
	switch k {
	case HistoryCellUser:
		return "user"
	case HistoryCellAssistant:
		return "assistant"
	case HistoryCellTool:
		return "tool"
	case HistoryCellSystem:
		return "system"
	case HistoryCellError:
		return "error"
	default:
		return "unknown"
	}
}

// HistoryCell is the renderable unit of TUI conversation history.
//
// Rich rendering and raw/copy-friendly output intentionally live behind the
// same abstraction so specialized cells can evolve without teaching the root
// Bubble Tea model about every presentation type.
type HistoryCell interface {
	Kind() HistoryCellKind
	Render() []string
	RawLines() []string
	LineCount() int
}

// widthHistoryCell is implemented by cells whose rich presentation can wrap
// to the current viewport. The compatibility methods on HistoryCell remain
// available to callers that do not have a terminal width.
type widthHistoryCell interface {
	RenderWidth(width int) []string
}

func renderHistoryCell(cell HistoryCell, width int) []string {
	if sized, ok := cell.(widthHistoryCell); ok {
		return sized.RenderWidth(width)
	}
	return cell.Render()
}

func historyCellLineCount(cell HistoryCell, width int) int {
	return len(renderHistoryCell(cell, width))
}

// UserCell renders submitted user input.
type UserCell struct{ Text string }

func (UserCell) Kind() HistoryCellKind { return HistoryCellUser }
func (c UserCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c UserCell) RenderWidth(width int) []string {
	lines := safeWrappedLines(strings.TrimRight(c.Text, "\n"), maxInt(1, width-2))
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for index, line := range lines {
		prefix := "  "
		if index == 0 {
			prefix = glyphMark
		}
		out = append(out, userStyle.Render(prefix)+bodyStyle.Render(line))
	}
	return out
}
func (c UserCell) RawLines() []string { return rawTextLines(c.Text) }
func (c UserCell) LineCount() int     { return len(c.RawLines()) }

// AssistantCell is mutable while assistant output is streaming.
type AssistantCell struct {
	Text          string
	renderCache   assistantRenderCache
	streamBuilder strings.Builder
	streamText    string
	streamActive  bool
}

type assistantRenderCache struct {
	width       int
	processed   int
	processedAt string
	lines       []string
	decorated   []string
	state       markdownRenderState
}

func (c *AssistantCell) appendDelta(delta string) {
	if delta == "" {
		return
	}
	if !c.streamActive || c.Text != c.streamText {
		c.streamBuilder.Reset()
		c.streamBuilder.Grow(len(c.Text) + len(delta))
		c.streamBuilder.WriteString(c.Text)
		c.streamActive = true
	}
	c.streamBuilder.WriteString(delta)
	c.Text = c.streamBuilder.String()
	c.streamText = c.Text
}

func (c *AssistantCell) sealStream() {
	if !c.streamActive {
		return
	}
	c.Text = strings.Clone(c.Text)
	c.streamBuilder.Reset()
	c.streamText = ""
	c.streamActive = false
	if c.renderCache.processed > 0 && c.renderCache.processed <= len(c.Text) {
		c.renderCache.processedAt = assistantCacheTail(c.Text[:c.renderCache.processed])
	}
}

func (*AssistantCell) Kind() HistoryCellKind { return HistoryCellAssistant }
func (c *AssistantCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c *AssistantCell) RenderWidth(width int) []string {
	text := assistantIncrementalText(c.Text)
	if text == "" {
		return nil
	}
	return c.renderAssistantIncremental(text, maxInt(8, width-2))
}

func assistantIncrementalText(text string) string {
	if !strings.HasSuffix(text, "\n") {
		return text
	}
	end := len(text) - 1
	for end > 0 && text[end-1] == '\n' {
		end--
	}
	return text[:end+1]
}

func (c *AssistantCell) renderAssistantIncremental(text string, width int) []string {
	if strings.ContainsRune(text, '\r') {
		c.renderCache = assistantRenderCache{}
		return decorateAssistantLines(renderMarkdownLines(text, width), 0)
	}
	cache, completeEnd := c.updateAssistantRenderCache(text, width)
	stableLen := len(cache.decorated)
	state := cache.state
	tail := text[completeEnd:]
	if tail == "" && !state.inFence {
		for stableLen > 0 && cache.lines[stableLen-1] == "" {
			stableLen--
		}
	}
	if tail == "" && !state.inFence {
		return cache.decorated[:stableLen]
	}
	out := append([]string(nil), cache.decorated[:stableLen]...)
	if tail != "" {
		tailLines := renderMarkdownLine(tail, width, &state)
		out = append(out, decorateAssistantLines(tailLines, stableLen)...)
	}
	if state.inFence {
		marker := markdownCodeStyle.Render("  └─ code (unterminated)")
		out = append(out, assistantDecoratedLine(marker, len(out)))
	}
	return out
}

func (c *AssistantCell) renderMarkdownIncremental(text string, width int) []string {
	if strings.ContainsRune(text, '\r') {
		c.renderCache = assistantRenderCache{}
		return renderMarkdownLines(text, width)
	}
	cache, completeEnd := c.updateAssistantRenderCache(text, width)
	out := append([]string(nil), cache.lines...)
	state := cache.state
	if tail := text[completeEnd:]; tail != "" {
		out = append(out, renderMarkdownLine(tail, width, &state)...)
	}
	if state.inFence {
		out = append(out, markdownCodeStyle.Render("  └─ code (unterminated)"))
	}
	return trimTrailingBlankLines(out)
}

func (c *AssistantCell) updateAssistantRenderCache(text string, width int) (*assistantRenderCache, int) {
	cache := &c.renderCache
	if cache.width != width || cache.processed > len(text) || !assistantCachePrefixMatches(text, cache) {
		*cache = assistantRenderCache{width: width}
	}
	completeEnd := strings.LastIndexByte(text, '\n') + 1
	if completeEnd < cache.processed {
		*cache = assistantRenderCache{width: width}
	}
	if cache.processed < completeEnd {
		segment := text[cache.processed:completeEnd]
		for offset := 0; offset < len(segment); {
			relativeEnd := strings.IndexByte(segment[offset:], '\n')
			if relativeEnd < 0 {
				break
			}
			end := offset + relativeEnd
			lines := renderMarkdownLine(segment[offset:end], width, &cache.state)
			start := len(cache.lines)
			cache.lines = append(cache.lines, lines...)
			cache.decorated = appendAssistantDecoratedLines(cache.decorated, lines, start)
			offset = end + 1
		}
		cache.processed = completeEnd
		cache.processedAt = assistantCacheTail(text[:completeEnd])
	}
	return cache, completeEnd
}

func appendAssistantDecoratedLines(dst []string, lines []string, start int) []string {
	for index, line := range lines {
		dst = append(dst, assistantDecoratedLine(line, start+index))
	}
	return dst
}

func decorateAssistantLines(lines []string, start int) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	for index, line := range lines {
		out[index] = assistantDecoratedLine(line, start+index)
	}
	return out
}

func assistantDecoratedLine(line string, index int) string {
	if index == 0 {
		return "● " + line
	}
	return "  " + line
}

func assistantCachePrefixMatches(text string, cache *assistantRenderCache) bool {
	if cache.processed == 0 || cache.processedAt == "" {
		return true
	}
	if cache.processed > len(text) || len(cache.processedAt) > cache.processed {
		return false
	}
	start := cache.processed - len(cache.processedAt)
	return text[start:cache.processed] == cache.processedAt
}

func assistantCacheTail(text string) string {
	const tailBytes = 64
	if len(text) <= tailBytes {
		return text
	}
	return text[len(text)-tailBytes:]
}

func (c *AssistantCell) RawLines() []string { return rawTextLines(c.Text) }
func (c *AssistantCell) LineCount() int     { return len(c.RawLines()) }

// AgentToolCell renders subagent lifecycle operations as orchestration rather
// than generic tool RPCs.
type AgentToolCell struct {
	CallID  string
	Name    string
	Target  string
	Summary string
	Running bool
	Spinner string
}

func (AgentToolCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c AgentToolCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c AgentToolCell) RenderWidth(width int) []string {
	label := c.presentationLabel(true)
	return wrapStyledLines(toolStyle.Render(glyphAgent)+mutedStyle.Render(sanitizeBubbleText(label)), maxInt(1, width))
}
func (c AgentToolCell) presentationLabel(includeSpinner bool) string {
	label := c.Summary
	if c.Running {
		switch c.Name {
		case "wait_agent":
			label = "Waiting for " + c.Target
		case "cancel_agent":
			label = "Canceling " + c.Target
		case "get_agent":
			label = "Checking " + c.Target
		case "list_agents":
			label = "Checking subagents"
		default:
			label = "Coordinating subagents"
		}
		if includeSpinner && c.Spinner != "" {
			label = c.Spinner + " " + label
		}
	}
	if label == "" {
		label = c.Name
	}
	return label
}
func (c AgentToolCell) RawLines() []string {
	return []string{sanitizeBubbleText(c.presentationLabel(false))}
}
func (c AgentToolCell) LineCount() int           { return 1 }
func (c AgentToolCell) historyToolID() string    { return c.CallID }
func (c AgentToolCell) historyToolName() string  { return c.Name }
func (c AgentToolCell) historyToolRunning() bool { return c.Running }

// ToolCell is the generic representation for a tool that has no specialized
// presentation model.
type ToolCell struct {
	CallID          string
	Name            string
	Body            string
	Stdout          string
	Stderr          string
	Running         bool
	ExitCode        *int
	Truncated       bool
	StdoutTruncated bool
	StderrTruncated bool
	Denied          bool
	FailureCode     tool.ErrorCode
	Spinner         string
	Target          string
	ToolKind        tool.Kind
	Summary         string
}

func (ToolCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c ToolCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c ToolCell) RenderWidth(width int) []string {
	var headerLine string

	if c.Running {
		glyph := toolKindGlyph(c.ToolKind, c.Name)
		indicator := " …"
		if c.Spinner != "" {
			indicator = " " + c.Spinner
		}
		targetStr := ""
		if strings.TrimSpace(c.Target) != "" {
			targetStr = " " + formatPathSegmentsStyled(c.Target)
		}
		headerLine = toolStyle.Render(glyph) + mutedStyle.Render(sanitizeBubbleText(c.Name)) + targetStr + toolStyle.Render(indicator)
	} else if c.Denied {
		targetStr := ""
		if strings.TrimSpace(c.Target) != "" {
			targetStr = " " + formatPathSegmentsStyled(c.Target)
		}
		headerLine = warningStyle.Render(glyphToolDenied) + mutedStyle.Render(sanitizeBubbleText(c.Name)) + targetStr + warningStyle.Render(glyphSep+"denied")
	} else if c.FailureCode != "" {
		targetStr := ""
		if strings.TrimSpace(c.Target) != "" {
			targetStr = " " + formatPathSegmentsStyled(c.Target)
		}
		headerLine = errorStyle.Render(glyphToolError) + mutedStyle.Render(sanitizeBubbleText(c.Name)) + targetStr + errorStyle.Render(glyphSep+string(c.FailureCode))
	} else if c.Name == "activate_skill" {
		if skillName := extractSkillContentName(c.Body); skillName != "" {
			headerLine = successStyle.Render(glyphToolSuccess) + mutedStyle.Render("Activated skill ") + toolTargetStyle.Render(fmt.Sprintf("%q", skillName))
		} else {
			headerLine = successStyle.Render(glyphToolSuccess) + mutedStyle.Render("activate_skill")
		}
	} else {
		summary := c.Summary
		if summary == "" && c.Body != "" {
			summary = summarizeToolOutput(c.Name, c.ToolKind, c.Target, c.Body, c.ExitCode, c.Truncated)
		}
		targetStr := ""
		if strings.TrimSpace(c.Target) != "" {
			targetStr = " " + formatPathSegmentsStyled(c.Target)
		}
		summaryStr := ""
		if summary != "" {
			summaryStr = toolSummaryStyle.Render(glyphSep + summary)
		}
		headerLine = successStyle.Render(glyphToolSuccess) + mutedStyle.Render(sanitizeBubbleText(c.Name)) + targetStr + summaryStr
	}

	out := make([]string, 0, 1)
	for _, line := range wrapStyledLines(headerLine, maxInt(1, width)) {
		out = append(out, line)
	}

	// Read file excerpt preview
	if !c.Running && c.Name == "read_file" && !c.Denied && c.FailureCode == "" && c.Body != "" {
		if excerpt := extractReadFileExcerpt(c.Body); excerpt != "" {
			out = append(out, toolExcerptStyle.Render("  ↳ "+excerpt))
		}
	}

	if !c.Running && !shouldSuppressBody(c.ToolKind, c.Name) {
		bodyLines := c.bodyLines()
		if len(bodyLines) > 0 {
			folded := formatOutputFold(bodyLines, 3)
			for _, line := range folded {
				for _, wrapped := range wrapStyledLines(line, maxInt(1, width-2)) {
					out = append(out, bodyStyle.Render("  "+wrapped))
				}
			}
		}
	}
	return out
}
func (c ToolCell) RawLines() []string {
	header := sanitizeBubbleText(c.Name)
	if c.Target != "" {
		header += " " + sanitizeBubbleText(c.Target)
	}
	out := []string{header}
	out = append(out, c.bodyLines()...)
	return out
}
func (c ToolCell) LineCount() int           { return len(c.RawLines()) }
func (c ToolCell) historyToolID() string    { return c.CallID }
func (c ToolCell) historyToolName() string  { return c.Name }
func (c ToolCell) historyToolRunning() bool { return c.Running }
func (c ToolCell) bodyLines() []string {
	if c.Name == "activate_skill" {
		return formatSkillToolBody(c.Body, c.ExitCode, c.Truncated, c.Denied, c.FailureCode)
	}
	return resultBodyLines(c.Body, c.ExitCode, c.Truncated, c.Denied, c.FailureCode)
}

func formatSkillToolBody(body string, exitCode *int, truncated bool, denied bool, failureCode tool.ErrorCode) []string {
	if denied {
		return []string{"denied"}
	}
	if failureCode != "" {
		return []string{"failure: " + string(failureCode)}
	}
	if skillName := extractSkillContentName(body); skillName != "" {
		return []string{fmt.Sprintf("[x] Activated skill %q", skillName)}
	}
	return resultBodyLines(body, exitCode, truncated, denied, failureCode)
}

func extractSkillContentName(body string) string {
	for _, quote := range []string{`name="`, `name='`} {
		idx := strings.Index(body, quote)
		if idx != -1 {
			rest := body[idx+len(quote):]
			end := strings.IndexAny(rest, `"'`)
			if end != -1 {
				return rest[:end]
			}
		}
	}
	return ""
}

// ExecCell gives shell execution a compact, command-oriented presentation.
type ExecCell struct {
	CallID          string
	Name            string
	Command         string
	Body            string
	Stdout          string
	Stderr          string
	Running         bool
	ExitCode        *int
	Truncated       bool
	StdoutTruncated bool
	StderrTruncated bool
	Denied          bool
	FailureCode     tool.ErrorCode
	Spinner         string
}

func (ExecCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c ExecCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c ExecCell) RenderWidth(width int) []string {
	command := strings.TrimSpace(c.Command)
	if command == "" {
		command = c.Name
	}
	var out []string
	var header string
	var headerStyle lipgloss.Style
	if c.Running {
		indicator := " …"
		if c.Spinner != "" {
			indicator = " " + c.Spinner
		}
		header = "$ " + sanitizeBubbleText(command) + indicator
		headerStyle = commandStyle
	} else if c.Denied {
		header = glyphToolDenied + "$ " + sanitizeBubbleText(command) + glyphSep + "denied"
		headerStyle = warningStyle
	} else if c.FailureCode != "" || (c.ExitCode != nil && *c.ExitCode != 0) {
		status := "failed"
		if c.ExitCode != nil {
			status = fmt.Sprintf("exit %d", *c.ExitCode)
		} else if c.FailureCode != "" {
			status = string(c.FailureCode)
		}
		header = glyphToolError + "$ " + sanitizeBubbleText(command) + glyphSep + status
		headerStyle = errorStyle
	} else {
		header := successStyle.Render(glyphToolSuccess) + commandStyle.Render("$ "+sanitizeBubbleText(command))
		if c.ExitCode != nil {
			header += toolSummaryStyle.Render(" (exit 0)")
		}
		out = make([]string, 0, 1)
		for _, line := range wrapStyledLines(header, maxInt(1, width)) {
			out = append(out, line)
		}
	}

	if c.Running || c.Denied || c.FailureCode != "" || (c.ExitCode != nil && *c.ExitCode != 0) {
		out = make([]string, 0, 1)
		for _, line := range safeWrappedLines(header, maxInt(1, width)) {
			out = append(out, headerStyle.Render(line))
		}
	}
	if !c.Running {
		for _, line := range c.renderOutputLines() {
			style := bodyStyle
			if strings.TrimSpace(line) == "stderr:" {
				style = warningStyle
			}
			for _, wrapped := range safeWrappedLines(line, maxInt(1, width-2)) {
				out = append(out, style.Render("  "+wrapped))
			}
		}
	}
	return out
}
func (c ExecCell) RawLines() []string {
	command := strings.TrimSpace(c.Command)
	if command == "" {
		command = c.Name
	}
	out := []string{"$ " + sanitizeBubbleText(command)}
	out = append(out, c.outputLines(true)...)
	return out
}
func (c ExecCell) renderOutputLines() []string {
	structured := c.Stdout != "" || c.Stderr != "" || c.StdoutTruncated || c.StderrTruncated
	if !structured {
		return formatOutputFold(resultBodyLines(c.Body, nil, c.Truncated, false, ""), 3)
	}
	out := make([]string, 0, 8)
	stdoutLines := rawTextLines(strings.TrimRight(c.Stdout, "\n"))
	if c.StdoutTruncated {
		stdoutLines = append(stdoutLines, "stdout truncated")
	}
	out = append(out, formatOutputFold(stdoutLines, 3)...)
	if c.Stderr != "" || c.StderrTruncated {
		out = append(out, "stderr:")
		stderrLines := rawTextLines(strings.TrimRight(c.Stderr, "\n"))
		if c.StderrTruncated {
			stderrLines = append(stderrLines, "stderr truncated")
		}
		out = append(out, formatOutputFold(stderrLines, 3)...)
	}
	if c.Truncated && !c.StdoutTruncated && !c.StderrTruncated {
		out = append(out, "output truncated")
	}
	return out
}

func (c ExecCell) outputLines(includeStatus bool) []string {
	structured := c.Stdout != "" || c.Stderr != "" || c.StdoutTruncated || c.StderrTruncated
	if !structured {
		exitCode := c.ExitCode
		denied := c.Denied
		failureCode := c.FailureCode
		if !includeStatus {
			exitCode, denied, failureCode = nil, false, ""
		}
		return resultBodyLines(c.Body, exitCode, c.Truncated, denied, failureCode)
	}
	lines := make([]string, 0, strings.Count(c.Stdout, "\n")+strings.Count(c.Stderr, "\n")+6)
	if stdout := strings.TrimRight(c.Stdout, "\n"); stdout != "" {
		lines = append(lines, rawTextLines(stdout)...)
	}
	if c.StdoutTruncated {
		lines = append(lines, "stdout truncated")
	}
	if stderr := strings.TrimRight(c.Stderr, "\n"); stderr != "" {
		lines = append(lines, "stderr:")
		for _, line := range rawTextLines(stderr) {
			lines = append(lines, "  "+line)
		}
	}
	if c.StderrTruncated {
		lines = append(lines, "stderr truncated")
	}
	if c.Truncated && !c.StdoutTruncated && !c.StderrTruncated {
		lines = append(lines, "output truncated")
	}
	if includeStatus {
		if c.ExitCode != nil {
			lines = append(lines, fmt.Sprintf("exit %d", *c.ExitCode))
		}
		if c.Denied {
			lines = append(lines, "denied")
		}
		if c.FailureCode != "" && !(c.ExitCode != nil && c.FailureCode == tool.ErrorCodeCommandFailed) {
			lines = append(lines, "failure: "+string(c.FailureCode))
		}
	}
	return lines
}

func (c ExecCell) LineCount() int           { return len(c.RawLines()) }
func (c ExecCell) historyToolID() string    { return c.CallID }
func (c ExecCell) historyToolName() string  { return c.Name }
func (c ExecCell) historyToolRunning() bool { return c.Running }

// PatchCell gives edit tools a change-oriented presentation. Paths are best
// effort presentation metadata derived from already-validated tool arguments;
// they never participate in authorization or execution.
type PatchCell struct {
	CallID      string
	Name        string
	Summary     string
	Paths       []string
	Body        string
	Running     bool
	Truncated   bool
	Denied      bool
	FailureCode tool.ErrorCode
	Spinner     string
}

func (PatchCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c PatchCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c PatchCell) RenderWidth(width int) []string {
	title := c.Name
	if strings.TrimSpace(c.Summary) != "" {
		title += " · " + c.Summary
	}
	var header string
	var headerStyle lipgloss.Style
	if c.Running {
		indicator := " …"
		if c.Spinner != "" {
			indicator = " " + c.Spinner
		}
		header = glyphEdit + title + indicator
		headerStyle = planStyle
	} else if c.Denied {
		header = glyphToolDenied + glyphEdit + title + glyphSep + "denied"
		headerStyle = warningStyle
	} else if c.FailureCode != "" {
		header = glyphToolError + glyphEdit + title + glyphSep + string(c.FailureCode)
		headerStyle = errorStyle
	} else {
		header = glyphToolSuccess + glyphEdit + title
		headerStyle = successStyle
	}
	out := make([]string, 0, 1)
	for _, line := range safeWrappedLines(header, maxInt(1, width)) {
		out = append(out, headerStyle.Render(line))
	}
	for _, path := range c.Paths {
		styledPath := formatPathSegmentsStyled(path)
		for _, wrapped := range wrapStyledLines(styledPath, maxInt(1, width-2)) {
			out = append(out, "  "+wrapped)
		}
	}
	if !c.Running && c.Body != "" {
		bodyLines := resultBodyLines(c.Body, nil, c.Truncated, c.Denied, c.FailureCode)
		if len(bodyLines) > 0 {
			folded := formatOutputFold(bodyLines, 3)
			for _, line := range folded {
				for _, wrapped := range safeWrappedLines(line, maxInt(1, width-2)) {
					out = append(out, bodyStyle.Render("  "+wrapped))
				}
			}
		}
	}
	return out
}
func (c PatchCell) RawLines() []string {
	title := c.Name
	if strings.TrimSpace(c.Summary) != "" {
		title += " · " + c.Summary
	}
	out := []string{glyphEdit + sanitizeBubbleText(title)}
	for _, path := range c.Paths {
		out = append(out, sanitizeBubbleText(path))
	}
	out = append(out, resultBodyLines(c.Body, nil, c.Truncated, c.Denied, c.FailureCode)...)
	return out
}
func (c PatchCell) LineCount() int           { return len(c.RawLines()) }
func (c PatchCell) historyToolID() string    { return c.CallID }
func (c PatchCell) historyToolName() string  { return c.Name }
func (c PatchCell) historyToolRunning() bool { return c.Running }

func resultBodyLines(body string, exitCode *int, truncated bool, denied bool, failureCode tool.ErrorCode) []string {
	body = strings.TrimRight(body, "\n")
	parts := make([]string, 0, strings.Count(body, "\n")+4)
	if body != "" {
		parts = append(parts, rawTextLines(body)...)
	}
	if exitCode != nil {
		parts = append(parts, fmt.Sprintf("exit %d", *exitCode))
	}
	if truncated {
		parts = append(parts, "output truncated")
	}
	if denied {
		parts = append(parts, "denied")
	}
	if failureCode != "" {
		parts = append(parts, "failure: "+string(failureCode))
	}
	return parts
}

// SystemCell renders low-priority status/history information.
type SystemCell struct{ Text string }

func (SystemCell) Kind() HistoryCellKind { return HistoryCellSystem }
func (c SystemCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c SystemCell) RenderWidth(width int) []string {
	return styledWrappedLines(c.Text, width, mutedStyle)
}
func (c SystemCell) RawLines() []string { return rawTextLines(c.Text) }
func (c SystemCell) LineCount() int     { return len(c.RawLines()) }

// ErrorCell renders a failed operation or classified OpenCode error.
type ErrorCell struct {
	Title       string
	Text        string
	Code        tool.ErrorCode
	ErrorKind   OpenCodeErrorKind
	Badge       string
	Suggestions []string
	RawDetails  string
	Retryable   bool
}

func (ErrorCell) Kind() HistoryCellKind { return HistoryCellError }
func (c ErrorCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }

func (c ErrorCell) RenderWidth(width int) []string {
	if width <= 0 {
		width = defaultBubbleWidth
	}

	// If this has structured error attributes (ErrorKind, Badge, or Suggestions),
	// render it as an OpenCode-style bordered error card.
	if c.Badge != "" || len(c.Suggestions) > 0 || (c.ErrorKind != "" && c.ErrorKind != ErrorKindGeneric) {
		return c.renderCard(width)
	}

	// Simple fallback rendering for legacy or simple tool errors
	text := c.Text
	if c.Title != "" {
		text = c.Title + ": " + text
	}
	return styledWrappedLines(glyphToolError+text, width, errorStyle)
}

func (c ErrorCell) renderCard(width int) []string {
	cardWidth := maxInt(24, width-2)
	innerWidth := cardWidth - 4 // Account for border (2) and padding (2)

	badge := c.Badge
	if badge == "" {
		badge = "ERROR"
	}
	title := c.Title
	if title == "" {
		title = "Error"
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentError).
		Render(fmt.Sprintf("%s[%s] %s", glyphToolError, badge, title))

	bodyLines := safeWrappedLines(c.Text, innerWidth)
	cardContent := []string{header}
	if len(bodyLines) > 0 {
		cardContent = append(cardContent, "")
		for _, bLine := range bodyLines {
			cardContent = append(cardContent, bodyStyle.Render(bLine))
		}
	}

	if len(c.Suggestions) > 0 {
		cardContent = append(cardContent, "")
		suggestHeader := lipgloss.NewStyle().Bold(true).Foreground(warningColor).Render("💡 Suggestions:")
		cardContent = append(cardContent, suggestHeader)
		for _, s := range c.Suggestions {
			wrappedS := safeWrappedLines("• "+s, innerWidth-2)
			for _, w := range wrappedS {
				cardContent = append(cardContent, mutedStyle.Render("  "+w))
			}
		}
	}

	joined := strings.Join(cardContent, "\n")
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentError).
		Padding(0, 1).
		Width(cardWidth)

	rendered := cardStyle.Render(joined)
	return strings.Split(rendered, "\n")
}

func (c ErrorCell) RawLines() []string {
	badge := c.Badge
	if badge == "" {
		badge = "ERROR"
	}
	title := c.Title
	if title == "" {
		title = "Error"
	}

	var lines []string
	if c.Badge != "" || len(c.Suggestions) > 0 || (c.ErrorKind != "" && c.ErrorKind != ErrorKindGeneric) {
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", badge, title, c.Text))
		for _, s := range c.Suggestions {
			lines = append(lines, "  • "+s)
		}
	} else {
		text := c.Text
		if c.Title != "" {
			text = c.Title + ": " + text
		}
		lines = append(lines, text)
	}
	return lines
}

func (c ErrorCell) LineCount() int { return len(c.RawLines()) }

// ThinkingCell represents an in-flight thought state in the transcript before
// any tokens stream from the model.
type ThinkingCell struct {
	Spinner string
}

func (ThinkingCell) Kind() HistoryCellKind { return HistoryCellAssistant }
func (c ThinkingCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c ThinkingCell) RenderWidth(_ int) []string {
	indicator := "…"
	if c.Spinner != "" {
		indicator = c.Spinner
	}
	return []string{assistantStyle.Render(indicator + " Thinking…")}
}
func (ThinkingCell) RawLines() []string { return []string{"Thinking…"} }
func (ThinkingCell) LineCount() int     { return 1 }

type runningHistoryTool interface {
	HistoryCell
	historyToolID() string
	historyToolName() string
	historyToolRunning() bool
}

// HistoryState separates finalized transcript cells from one mutable in-flight
// cell. Renderers always see committed cells plus the live active tail.
type HistoryState struct {
	committed        []HistoryCell
	active           HistoryCell
	maxLines         int
	renderWidth      int
	cachedRender     []string
	cachedRenderText string
	renderTextValid  bool
	cachedRaw        []string
	cachedRawText    string
	rawTextValid     bool
	cacheValid       bool
	cachedWidth      int
	altRender        []string
	altRenderValid   bool
	altRenderWidth   int
	spinnerFrame     string
	committedLines   int
}

func NewHistoryState(maxLines int) *HistoryState {
	if maxLines <= 0 {
		maxLines = maxBubbleScrollback
	}
	return &HistoryState{committed: make([]HistoryCell, 0), maxLines: maxLines, renderWidth: defaultBubbleWidth}
}

// SetWidth updates the rich transcript width and invalidates visual caches.
// Raw transcript consumers remain independent of terminal dimensions.
func (s *HistoryState) SetWidth(width int) {
	if s == nil {
		return
	}
	if width <= 0 {
		width = defaultBubbleWidth
	}
	if s.renderWidth == width {
		return
	}
	s.renderWidth = width
	s.cacheValid = false
	s.buildCommittedCache()
}

func (s *HistoryState) SetSpinnerFrame(frame string) bool {
	if s == nil {
		return false
	}
	s.spinnerFrame = frame
	changed := cellUsesSpinner(s.active)
	if changed {
		setCellSpinner(s.active, frame)
	}
	for _, cell := range s.committed {
		if r, ok := cell.(runningHistoryTool); ok && r.historyToolRunning() {
			setCellSpinner(cell, frame)
			s.cacheValid = false
			s.altRenderValid = false
			changed = true
		}
	}
	return changed
}

func cellUsesSpinner(cell HistoryCell) bool {
	switch typed := cell.(type) {
	case *ToolCell:
		return typed.Running
	case *AgentToolCell:
		return typed.Running
	case *ExecCell:
		return typed.Running
	case *PatchCell:
		return typed.Running
	case *ThinkingCell:
		return true
	default:
		return false
	}
}

func (s *HistoryState) SpinnerFrame() string {
	if s == nil {
		return ""
	}
	return s.spinnerFrame
}

func setCellSpinner(cell HistoryCell, frame string) {
	switch typed := cell.(type) {
	case *ToolCell:
		typed.Spinner = frame
	case *AgentToolCell:
		typed.Spinner = frame
	case *ExecCell:
		typed.Spinner = frame
	case *PatchCell:
		typed.Spinner = frame
	case *ThinkingCell:
		typed.Spinner = frame
	}
}

func (s *HistoryState) Cells() []HistoryCell {
	cells := append([]HistoryCell{}, s.committed...)
	if s.active != nil {
		cells = append(cells, s.active)
	}
	return cells
}

func (s *HistoryState) Committed() []HistoryCell {
	return append([]HistoryCell{}, s.committed...)
}

func (s *HistoryState) Active() HistoryCell { return s.active }

func (s *HistoryState) Append(cell HistoryCell) {
	if cell == nil {
		return
	}
	s.CommitActive()
	s.committed = append(s.committed, cell)
	s.committedLines += historyCellLineCount(cell, s.renderWidth)
	s.cacheValid = false
	s.altRenderValid = false
	s.trim()
}

func (s *HistoryState) StartThinking() {
	s.CommitActive()
	s.active = &ThinkingCell{Spinner: s.spinnerFrame}
}

func (s *HistoryState) AppendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if _, ok := s.active.(*ThinkingCell); ok {
		cell := &AssistantCell{}
		cell.appendDelta(delta)
		s.active = cell
		return
	}
	if assistant, ok := s.active.(*AssistantCell); ok {
		assistant.appendDelta(delta)
		return
	}
	s.CommitActive()
	cell := &AssistantCell{}
	cell.appendDelta(delta)
	s.active = cell
}

func (s *HistoryState) StartTool(name string) {
	s.StartToolCell(&ToolCell{Name: name, Running: true})
}

func (s *HistoryState) StartToolCall(callID string, name string) {
	s.StartToolCell(&ToolCell{CallID: callID, Name: name, Running: true})
}

func (s *HistoryState) StartToolCell(cell HistoryCell) {
	if cell == nil {
		return
	}
	s.CommitActive()
	if s.spinnerFrame != "" {
		setCellSpinner(cell, s.spinnerFrame)
	}
	s.active = cell
}

func (s *HistoryState) CompleteTool(completed ToolCell) {
	completed.Running = false
	s.CompleteToolCall(completed.CallID, completed.Name, &completed)
}

// CompleteToolCell is the name-based compatibility path used by legacy tests.
// Runtime tool events should use CompleteToolCall so parallel calls of the same
// tool cannot be confused.
func (s *HistoryState) CompleteToolCell(name string, completed HistoryCell) {
	s.CompleteToolCall("", name, completed)
}

func (s *HistoryState) CompleteToolCall(callID string, name string, completed HistoryCell) {
	if completed == nil {
		return
	}
	if runningToolMatches(s.active, callID, name) {
		s.active = completed
		s.CommitActive()
		return
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		if !runningToolMatches(s.committed[i], callID, name) {
			continue
		}
		s.committedLines -= historyCellLineCount(s.committed[i], s.renderWidth)
		s.committed[i] = completed
		s.committedLines += historyCellLineCount(completed, s.renderWidth)
		s.cacheValid = false
		s.altRenderValid = false
		s.trim()
		return
	}
	s.Append(completed)
}

func runningToolMatches(cell HistoryCell, callID string, name string) bool {
	running, ok := cell.(runningHistoryTool)
	if !ok || !running.historyToolRunning() {
		return false
	}
	if callID != "" {
		return running.historyToolID() == callID
	}
	return name != "" && running.historyToolName() == name
}

func (s *HistoryState) CommitActive() {
	if s.active == nil {
		return
	}
	if assistant, ok := s.active.(*AssistantCell); ok {
		assistant.sealStream()
	}
	if _, ok := s.active.(*ThinkingCell); ok {
		s.active = nil
		return
	}
	s.committed = append(s.committed, s.active)
	s.committedLines += historyCellLineCount(s.active, s.renderWidth)
	s.active = nil
	s.cacheValid = false
	s.altRenderValid = false
	s.trim()
}

func (s *HistoryState) Reset() {
	s.committed = s.committed[:0]
	s.active = nil
	s.cacheValid = false
	s.altRenderValid = false
	s.cachedRender = nil
	s.cachedRenderText = ""
	s.renderTextValid = false
	s.altRender = nil
	s.cachedRaw = nil
	s.cachedRawText = ""
	s.rawTextValid = false
	s.committedLines = 0
}

func (s *HistoryState) InvalidateCache() {
	s.cacheValid = false
	s.altRenderValid = false
}

func (s *HistoryState) buildCommittedCache() {
	if s.cacheValid && s.cachedWidth == s.renderWidth {
		return
	}
	render := make([]string, 0, len(s.committed)*4)
	raw := make([]string, 0, len(s.committed)*2)
	committedLines := 0
	for index, cell := range s.committed {
		if index > 0 {
			render = append(render, "")
		}
		cellLines := renderHistoryCell(cell, s.renderWidth)
		committedLines += len(cellLines)
		render = append(render, cellLines...)
		raw = append(raw, cell.RawLines()...)
	}
	s.committedLines = committedLines
	s.cachedRender = render
	s.cachedRenderText = ""
	s.renderTextValid = false
	s.cachedRaw = raw
	s.cachedRawText = ""
	s.rawTextValid = false
	s.cachedWidth = s.renderWidth
	s.cacheValid = true
}

func (s *HistoryState) RenderLines() []string {
	s.buildCommittedCache()
	if s.active == nil {
		return append([]string(nil), s.cachedRender...)
	}
	activeLines := renderHistoryCell(s.active, s.renderWidth)
	out := make([]string, len(s.cachedRender), len(s.cachedRender)+len(activeLines)+1)
	copy(out, s.cachedRender)
	if len(out) > 0 {
		out = append(out, "")
	}
	return append(out, activeLines...)
}

// RenderContent renders the main transcript directly as viewport content.
// The finalized prefix is cached so streaming updates only rebuild the active tail.
func (s *HistoryState) RenderContent() string {
	if s == nil {
		return ""
	}
	s.buildCommittedCache()
	committed := s.committedRenderText()
	if s.active == nil {
		return committed
	}
	activeLines := renderHistoryCell(s.active, s.renderWidth)
	if len(activeLines) == 0 {
		return committed
	}
	activeBytes := len(activeLines) - 1
	for _, line := range activeLines {
		activeBytes += len(line)
	}
	separatorBytes := 0
	if committed != "" {
		separatorBytes = 2
	}
	var out strings.Builder
	out.Grow(len(committed) + separatorBytes + activeBytes)
	if committed != "" {
		out.WriteString(committed)
		out.WriteString("\n\n")
	}
	for index, line := range activeLines {
		if index > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	return out.String()
}

func (s *HistoryState) committedRenderText() string {
	if s.renderTextValid {
		return s.cachedRenderText
	}
	s.cachedRenderText = strings.Join(s.cachedRender, "\n")
	s.renderTextValid = true
	return s.cachedRenderText
}

// RenderTailContent renders only the newest rich transcript lines. It reports
// whether older lines were omitted so callers can hydrate full scrollback on demand.
func (s *HistoryState) RenderTailContent(maxLines int) (string, bool) {
	if s == nil || maxLines <= 0 {
		return s.RenderContent(), false
	}
	s.buildCommittedCache()
	var activeLines []string
	if s.active != nil {
		activeLines = renderHistoryCell(s.active, s.renderWidth)
	}
	separator := len(s.cachedRender) > 0 && len(activeLines) > 0
	totalLines := len(s.cachedRender) + len(activeLines)
	if separator {
		totalLines++
	}
	if totalLines <= maxLines {
		return s.RenderContent(), false
	}

	remaining := maxLines
	activeStart := len(activeLines)
	if remaining > 0 && len(activeLines) > 0 {
		take := minInt(remaining, len(activeLines))
		activeStart -= take
		remaining -= take
	}
	includeSeparator := false
	if remaining > 0 && separator && activeStart == 0 {
		includeSeparator = true
		remaining--
	}
	committedStart := len(s.cachedRender)
	if remaining > 0 {
		take := minInt(remaining, len(s.cachedRender))
		committedStart -= take
	}
	return joinRenderedTail(s.cachedRender[committedStart:], includeSeparator, activeLines[activeStart:]), true
}

func joinRenderedTail(committed []string, blankSeparator bool, active []string) string {
	bytes := 0
	for _, line := range committed {
		bytes += len(line)
	}
	for _, line := range active {
		bytes += len(line)
	}
	if len(committed) > 1 {
		bytes += len(committed) - 1
	}
	if len(active) > 1 {
		bytes += len(active) - 1
	}
	if blankSeparator {
		bytes += 2
	}
	var out strings.Builder
	out.Grow(bytes)
	for index, line := range committed {
		if index > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	if blankSeparator {
		out.WriteString("\n\n")
	}
	for index, line := range active {
		if index > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	return out.String()
}

// RenderLinesAt renders rich content at a temporary width, useful for the
// narrower transcript overlay without changing the main viewport's cache.
func (s *HistoryState) RenderLinesAt(width int) []string {
	if s == nil {
		return nil
	}
	if width <= 0 {
		width = s.renderWidth
	}
	if width == s.renderWidth {
		return s.RenderLines()
	}
	s.buildAlternateRenderCache(width)
	if s.active == nil {
		return append([]string(nil), s.altRender...)
	}
	activeLines := renderHistoryCell(s.active, width)
	out := make([]string, len(s.altRender), len(s.altRender)+len(activeLines)+1)
	copy(out, s.altRender)
	if len(out) > 0 {
		out = append(out, "")
	}
	return append(out, activeLines...)
}

func (s *HistoryState) buildAlternateRenderCache(width int) {
	if s.altRenderValid && s.altRenderWidth == width {
		return
	}
	render := make([]string, 0, len(s.committed)*4)
	for index, cell := range s.committed {
		if index > 0 {
			render = append(render, "")
		}
		render = append(render, renderHistoryCell(cell, width)...)
	}
	s.altRender = render
	s.altRenderWidth = width
	s.altRenderValid = true
}

func (s *HistoryState) Raw() string {
	s.buildCommittedCache()
	committedRaw := s.committedRawText()
	if s.active == nil {
		return committedRaw
	}
	activeRaw := strings.Join(s.active.RawLines(), "\n")
	if committedRaw == "" {
		return activeRaw
	}
	if activeRaw == "" {
		return committedRaw
	}
	var out strings.Builder
	out.Grow(len(committedRaw) + 1 + len(activeRaw))
	out.WriteString(committedRaw)
	out.WriteByte('\n')
	out.WriteString(activeRaw)
	return out.String()
}

func (s *HistoryState) committedRawText() string {
	if s.rawTextValid {
		return s.cachedRawText
	}
	s.cachedRawText = strings.Join(s.cachedRaw, "\n")
	s.rawTextValid = true
	return s.cachedRawText
}

func (s *HistoryState) trim() {
	activeCount := 0
	if s.active != nil {
		activeCount = historyCellLineCount(s.active, s.renderWidth)
	}
	for s.committedLines+activeCount > s.maxLines && len(s.committed) > 1 {
		popped := s.committed[0]
		s.committed = s.committed[1:]
		s.committedLines -= historyCellLineCount(popped, s.renderWidth)
		s.cacheValid = false
		s.altRenderValid = false
	}
	if s.committedLines < 0 {
		s.committedLines = 0
	}
}

func (s *HistoryState) lineCount() int {
	total := s.committedLines
	if s.active != nil {
		total += historyCellLineCount(s.active, s.renderWidth)
	}
	return total
}

func rawTextLines(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = sanitizeBubbleText(lines[i])
	}
	return lines
}

func renderStyledLines(text string, style func(string) string) []string {
	lines := rawTextLines(text)
	for i := range lines {
		lines[i] = style(lines[i])
	}
	return lines
}

func safeWrappedLines(text string, width int) []string {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if text == "" {
		return nil
	}
	lines := make([]string, 0, strings.Count(text, "\n")+1)
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, wrapLines(sanitizeBubbleText(line), width)...)
	}
	return lines
}

func wrapStyledLines(styledText string, width int) []string {
	styledText = strings.TrimRight(strings.ReplaceAll(styledText, "\r\n", "\n"), "\n")
	if styledText == "" {
		return nil
	}
	lines := make([]string, 0, strings.Count(styledText, "\n")+1)
	for _, line := range strings.Split(styledText, "\n") {
		lines = append(lines, wrapLines(line, width)...)
	}
	return lines
}

func styledWrappedLines(text string, width int, style lipgloss.Style) []string {
	lines := safeWrappedLines(text, width)
	for index := range lines {
		lines[index] = style.Render(lines[index])
	}
	return lines
}
