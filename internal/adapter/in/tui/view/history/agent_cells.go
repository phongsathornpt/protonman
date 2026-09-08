package history

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/execview"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

// AgentRunCell presents one delegated subagent job as a single user-facing
// lifecycle instead of exposing orchestration RPCs such as wait_agent.
type AgentRunCell struct {
	AgentID    string
	Profile    agent.Profile
	Task       string
	State      agent.State
	Activity   string
	Summary    string
	Reason     string
	StartedAt  time.Time
	FinishedAt time.Time
	Spinner    string
}

func (AgentRunCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c AgentRunCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }
func (c AgentRunCell) RenderWidth(width int) []string {
	label := c.title()
	indicator, style := c.statePresentation()
	header := style.Render(indicator + label)
	if duration := c.duration(); duration > 0 {
		header += tuistyle.ToolSummaryStyle.Render(tuistyle.GlyphSep + execview.FormatDuration(duration))
	}
	out := wrapStyledLines(header, max(1, width))
	if detail := c.detail(); detail != "" {
		for _, line := range wrapStyledLines(tuistyle.BodyStyle.Render("  "+detail), max(1, width)) {
			out = append(out, line)
		}
	}
	return out
}

func (c AgentRunCell) RawLines() []string {
	out := []string{sanitizeBubbleText(c.title())}
	if detail := c.detail(); detail != "" {
		out = append(out, sanitizeBubbleText(detail))
	}
	return out
}

func (c AgentRunCell) LineCount() int { return len(c.RawLines()) }
func (c AgentRunCell) title() string {
	profile := c.Profile.ShortLabel()
	if !c.Profile.Valid() {
		profile = "AGENT"
	}
	task := strings.TrimSpace(c.Task)
	if task == "" {
		task = strings.TrimSpace(c.AgentID)
	}
	if task == "" {
		return profile
	}
	return profile + " " + task
}

func (c AgentRunCell) detail() string {
	if c.State == agent.StateFailed || c.State == agent.StateCanceled {
		if reason := strings.TrimSpace(c.Reason); reason != "" {
			return reason
		}
	}
	if c.State.Terminal() {
		if summary := strings.TrimSpace(c.Summary); summary != "" {
			return summary
		}
		switch c.State {
		case agent.StateCanceled:
			return "canceled"
		case agent.StateFailed:
			return "failed"
		}
	}
	return strings.TrimSpace(c.Activity)
}
func (c AgentRunCell) duration() time.Duration {
	if c.StartedAt.IsZero() {
		return 0
	}
	end := c.FinishedAt
	if end.IsZero() {
		return 0
	}
	if end.Before(c.StartedAt) {
		return 0
	}
	return end.Sub(c.StartedAt)
}

func (c AgentRunCell) statePresentation() (string, lipgloss.Style) {
	switch c.State {
	case agent.StateCompleted:
		return tuistyle.GlyphToolSuccess, tuistyle.SuccessStyle
	case agent.StateFailed, agent.StateCanceled:
		return tuistyle.GlyphToolError, tuistyle.ErrorStyle
	case agent.StateCanceling:
		return tuistyle.GlyphAgent, tuistyle.WarningStyle
	case agent.StateQueued:
		return "○ ", tuistyle.MutedStyle
	default:
		indicator := tuistyle.GlyphAgent
		if c.Spinner != "" {
			indicator = c.Spinner + " "
		}
		return indicator, tuistyle.ToolStyle
	}
}

func (c AgentRunCell) String() string {
	return fmt.Sprintf("%s:%s", c.AgentID, c.State)
}
