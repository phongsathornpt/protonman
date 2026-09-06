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
type AssistantCell struct{ Text string }

func (AssistantCell) Kind() HistoryCellKind { return HistoryCellAssistant }
func (c AssistantCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c AssistantCell) RenderWidth(width int) []string {
	text := strings.TrimRight(c.Text, "\n")
	if text == "" {
		return nil
	}
	lines := renderMarkdownLines(text, maxInt(8, width-2))
	out := make([]string, 0, len(lines))
	for index, line := range lines {
		prefix := "  "
		if index == 0 {
			prefix = "● "
		}
		out = append(out, prefix+line)
	}
	return out
}
func (c AssistantCell) RawLines() []string { return rawTextLines(c.Text) }
func (c AssistantCell) LineCount() int     { return len(c.RawLines()) }

// ToolCell is the generic representation for a tool that has no specialized
// presentation model.
type ToolCell struct {
	CallID      string
	Name        string
	Body        string
	Running     bool
	ExitCode    *int
	Truncated   bool
	Denied      bool
	FailureCode tool.ErrorCode
	Spinner     string
	Target      string
	ToolKind    tool.Kind
	Summary     string
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
	CallID      string
	Name        string
	Command     string
	Body        string
	Running     bool
	ExitCode    *int
	Truncated   bool
	Denied      bool
	FailureCode tool.ErrorCode
	Spinner     string
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
		bodyLines := resultBodyLines(c.Body, nil, c.Truncated, false, "")
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
func (c ExecCell) RawLines() []string {
	command := strings.TrimSpace(c.Command)
	if command == "" {
		command = c.Name
	}
	out := []string{"$ " + sanitizeBubbleText(command)}
	out = append(out, resultBodyLines(c.Body, c.ExitCode, c.Truncated, c.Denied, c.FailureCode)...)
	return out
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
	committed      []HistoryCell
	active         HistoryCell
	maxLines       int
	renderWidth    int
	cachedRender   []string
	cachedRaw      []string
	cacheValid     bool
	cachedWidth    int
	altRender      []string
	altRenderValid bool
	altRenderWidth int
	spinnerFrame   string
	committedLines int
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

func (s *HistoryState) SetSpinnerFrame(frame string) {
	if s == nil {
		return
	}
	s.spinnerFrame = frame
	if s.active != nil {
		setCellSpinner(s.active, frame)
	}
	for _, cell := range s.committed {
		if r, ok := cell.(runningHistoryTool); ok && r.historyToolRunning() {
			setCellSpinner(cell, frame)
			s.cacheValid = false
			s.altRenderValid = false
		}
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
		s.active = &AssistantCell{Text: delta}
		return
	}
	if assistant, ok := s.active.(*AssistantCell); ok {
		assistant.Text += delta
		return
	}
	s.CommitActive()
	s.active = &AssistantCell{Text: delta}
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
	s.altRender = nil
	s.cachedRaw = nil
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
	s.cachedRaw = raw
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
	if s.active == nil {
		return strings.Join(s.cachedRaw, "\n")
	}
	out := make([]string, len(s.cachedRaw), len(s.cachedRaw)+len(s.active.RawLines()))
	copy(out, s.cachedRaw)
	out = append(out, s.active.RawLines()...)
	return strings.Join(out, "\n")
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
