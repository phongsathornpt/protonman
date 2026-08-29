package tui

import (
	"fmt"
	"strings"
)

// HistoryCellKind identifies the semantic role of one transcript cell.
type HistoryCellKind uint8

const (
	HistoryCellUser HistoryCellKind = iota
	HistoryCellAssistant
	HistoryCellTool
	HistoryCellSystem
	HistoryCellError
)

// HistoryCell is the renderable unit of TUI conversation history.
//
// Rich rendering and raw/copy-friendly output intentionally live behind the
// same abstraction so specialized cells (exec, patch, plan, MCP) can evolve
// without teaching the root Bubble Tea model about every presentation type.
type HistoryCell interface {
	Kind() HistoryCellKind
	Render() []string
	RawLines() []string
	LineCount() int
}

// UserCell renders submitted user input.
type UserCell struct{ Text string }

func (UserCell) Kind() HistoryCellKind { return HistoryCellUser }
func (c UserCell) Render() []string {
	return []string{userStyle.Render(glyphMark) + bodyStyle.Render(sanitizeBubbleText(c.Text))}
}
func (c UserCell) RawLines() []string { return rawTextLines(c.Text) }
func (c UserCell) LineCount() int      { return len(c.RawLines()) }

// AssistantCell is mutable while assistant output is streaming.
type AssistantCell struct{ Text string }

func (AssistantCell) Kind() HistoryCellKind { return HistoryCellAssistant }
func (c AssistantCell) Render() []string {
	text := strings.TrimRight(c.Text, "\n")
	if text == "" {
		return nil
	}
	out := make([]string, 0, strings.Count(text, "\n")+1)
	for i, line := range strings.Split(text, "\n") {
		prefix := "  "
		if i == 0 {
			prefix = ""
		}
		out = append(out, assistantStyle.Render(prefix+sanitizeBubbleText(line)))
	}
	return out
}
func (c AssistantCell) RawLines() []string { return rawTextLines(c.Text) }
func (c AssistantCell) LineCount() int      { return len(c.RawLines()) }

// ToolCell represents one tool execution. It may be committed while still
// running when another parallel tool becomes the active cell.
type ToolCell struct {
	Name        string
	Body        string
	Running     bool
	ExitCode    *int
	Truncated   bool
	Denied      bool
	FailureCode string
}

func (ToolCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c ToolCell) Render() []string {
	header := glyphTool + c.Name
	if c.Running {
		header += " …"
	}
	out := []string{toolStyle.Render(header)}
	for _, line := range c.bodyLines() {
		out = append(out, bodyStyle.Render("  "+sanitizeBubbleText(line)))
	}
	return out
}
func (c ToolCell) RawLines() []string {
	out := []string{sanitizeBubbleText(c.Name)}
	out = append(out, c.bodyLines()...)
	return out
}
func (c ToolCell) LineCount() int { return len(c.RawLines()) }
func (c ToolCell) bodyLines() []string {
	body := strings.TrimRight(c.Body, "\n")
	parts := make([]string, 0, strings.Count(body, "\n")+4)
	if body != "" {
		parts = append(parts, rawTextLines(body)...)
	}
	if c.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("exit %d", *c.ExitCode))
	}
	if c.Truncated {
		parts = append(parts, "output truncated")
	}
	if c.Denied {
		parts = append(parts, "denied")
	}
	if c.FailureCode != "" {
		parts = append(parts, "failure: "+c.FailureCode)
	}
	return parts
}

// SystemCell renders low-priority status/history information.
type SystemCell struct{ Text string }

func (SystemCell) Kind() HistoryCellKind { return HistoryCellSystem }
func (c SystemCell) Render() []string {
	return renderStyledLines(c.Text, func(line string) string { return mutedStyle.Render(line) })
}
func (c SystemCell) RawLines() []string { return rawTextLines(c.Text) }
func (c SystemCell) LineCount() int      { return len(c.RawLines()) }

// ErrorCell renders a failed operation.
type ErrorCell struct {
	Title string
	Text  string
	Code  string
}

func (ErrorCell) Kind() HistoryCellKind { return HistoryCellError }
func (c ErrorCell) Render() []string {
	text := c.Text
	if c.Title != "" {
		text = c.Title + ": " + text
	}
	return renderStyledLines(text, func(line string) string { return errorStyle.Render(line) })
}
func (c ErrorCell) RawLines() []string {
	text := c.Text
	if c.Title != "" {
		text = c.Title + ": " + text
	}
	return rawTextLines(text)
}
func (c ErrorCell) LineCount() int { return len(c.RawLines()) }

// HistoryState separates finalized transcript cells from one mutable in-flight
// cell. This mirrors the interaction model used by modern coding-agent TUIs
// while remaining independent of the model/tool execution layer.
type HistoryState struct {
	committed []HistoryCell
	active    HistoryCell
	maxLines  int
}

func NewHistoryState(maxLines int) *HistoryState {
	if maxLines <= 0 {
		maxLines = maxBubbleScrollback
	}
	return &HistoryState{committed: make([]HistoryCell, 0), maxLines: maxLines}
}

func (s *HistoryState) Cells() []HistoryCell {
	cells := append([]HistoryCell{}, s.committed...)
	if s.active != nil {
		cells = append(cells, s.active)
	}
	return cells
}

func (s *HistoryState) Active() HistoryCell { return s.active }

func (s *HistoryState) Append(cell HistoryCell) {
	if cell == nil {
		return
	}
	s.CommitActive()
	s.committed = append(s.committed, cell)
	s.trim()
}

func (s *HistoryState) AppendAssistantDelta(delta string) {
	if delta == "" {
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
	s.CommitActive()
	s.active = &ToolCell{Name: name, Running: true}
}

func (s *HistoryState) CompleteTool(completed ToolCell) {
	completed.Running = false
	if active, ok := s.active.(*ToolCell); ok && active.Name == completed.Name {
		s.active = &completed
		s.CommitActive()
		return
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		running, ok := s.committed[i].(*ToolCell)
		if !ok || !running.Running || running.Name != completed.Name {
			continue
		}
		s.committed[i] = &completed
		s.trim()
		return
	}
	s.Append(&completed)
}

func (s *HistoryState) CommitActive() {
	if s.active == nil {
		return
	}
	s.committed = append(s.committed, s.active)
	s.active = nil
	s.trim()
}

func (s *HistoryState) Reset() {
	s.committed = s.committed[:0]
	s.active = nil
}

func (s *HistoryState) RenderLines() []string {
	out := make([]string, 0)
	for _, cell := range s.Cells() {
		out = append(out, cell.Render()...)
	}
	return out
}

func (s *HistoryState) Raw() string {
	out := make([]string, 0)
	for _, cell := range s.Cells() {
		out = append(out, cell.RawLines()...)
	}
	return strings.Join(out, "\n")
}

func (s *HistoryState) trim() {
	for s.lineCount() > s.maxLines && len(s.committed) > 1 {
		s.committed = s.committed[1:]
	}
}

func (s *HistoryState) lineCount() int {
	total := 0
	for _, cell := range s.committed {
		total += cell.LineCount()
	}
	if s.active != nil {
		total += s.active.LineCount()
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
