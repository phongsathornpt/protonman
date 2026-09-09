package history

import (
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

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
func (c AgentToolCell) RenderWidth(width int) []string {
	label := c.presentationLabel(true)
	if c.Running {
		return wrapStyledLines(tuistyle.ToolStyle.Render(tuistyle.GlyphAgent)+tuistyle.MutedStyle.Render(sanitizeBubbleText(label)), max(1, width))
	}
	return wrapStyledLines(tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess)+tuistyle.MutedStyle.Render(sanitizeBubbleText(label)), max(1, width))
}
func (c AgentToolCell) presentationLabel(includeSpinner bool) string {
	label := c.Summary
	if c.Running {
		label = "Coordinating subagents"
		if includeSpinner && c.Spinner != "" {
			label = c.Spinner + " " + label
		}
	}
	if label == "" {
		label = tool.DisplayName(c.Name)
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
