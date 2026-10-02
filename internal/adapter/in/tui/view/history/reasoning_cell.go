package history

import (
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/execview"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

// ReasoningCell presents model thinking/reasoning as a collapsible, duration-aware cell.
type ReasoningCell struct {
	Content     string
	StartedAt   time.Time
	Duration    time.Duration
	TokenCount  int
	Streaming   bool
	Expanded    bool
	Spinner     string
	Icons       tuistyle.IconSet
	Highlighted bool
}

func (ReasoningCell) Kind() HistoryCellKind { return HistoryCellReasoning }

func (c *ReasoningCell) IsExpanded() bool          { return c.Expanded }
func (c *ReasoningCell) SetExpanded(expanded bool) { c.Expanded = expanded }
func (c *ReasoningCell) ToggleExpanded()           { c.Expanded = !c.Expanded }
func (c *ReasoningCell) CanExpand() bool           { return len(strings.TrimSpace(c.Content)) > 0 }

func (c *ReasoningCell) elapsedDuration() time.Duration {
	if c.Duration > 0 {
		return c.Duration
	}
	if !c.StartedAt.IsZero() {
		return time.Since(c.StartedAt)
	}
	return 0
}

func (c *ReasoningCell) summaryLabel() string {
	duration := c.elapsedDuration()
	durationStr := ""
	if duration > 0 {
		durationStr = execview.FormatDuration(duration)
	}
	var parts []string
	if durationStr != "" {
		parts = append(parts, "for "+durationStr)
	}
	if c.TokenCount > 0 {
		parts = append(parts, fmt.Sprintf("%d tokens", c.TokenCount))
	}
	if len(parts) == 0 {
		return "Thought"
	}
	return "Thought " + strings.Join(parts, " · ")
}

func (c *ReasoningCell) RenderWidth(width int) []string {
	width = max(1, width)
	icons := tuistyle.OrUnicodeIcons(c.Icons)

	if c.Streaming {
		// During streaming, show active thinking header with spinner
		duration := c.elapsedDuration()
		durationStr := ""
		if duration > 0 {
			durationStr = " (" + execview.FormatDuration(duration) + ")"
		}
		indicator := ""
		if c.Spinner != "" {
			indicator = " " + c.Spinner
		}
		header := tuistyle.ActivityStyle.Render(icons.Thought+"Thinking"+durationStr) + indicator
		out := wrapStyledLines(header, width)
		if c.Expanded && strings.TrimSpace(c.Content) != "" {
			lines := strings.Split(strings.TrimSpace(c.Content), "\n")
			for _, line := range lines {
				for _, wrapped := range safeWrappedLines(line, max(1, width-4)) {
					out = append(out, tuistyle.ThoughtBodyStyle.Render("  │ "+wrapped))
				}
			}
		}
		return out
	}

	// Completed state: check if collapsed or expanded
	foldIcon := icons.FoldCollapsed
	if c.Expanded {
		foldIcon = icons.FoldExpanded
	}

	summary := c.summaryLabel()
	headerText := foldIcon + summary
	if c.Highlighted {
		headerText = "» " + headerText
	}

	styledHeader := tuistyle.ThoughtHeaderStyle.Render(headerText)
	if c.Highlighted {
		styledHeader = tuistyle.NavHighlightStyle.Render(headerText)
	}

	out := wrapStyledLines(styledHeader, width)
	if !c.Expanded || strings.TrimSpace(c.Content) == "" {
		return out
	}

	// Render expanded content with subtle border
	content := strings.TrimSpace(c.Content)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			out = append(out, tuistyle.DiffFrameStyle.Render("  │"))
			continue
		}
		for _, wrapped := range safeWrappedLines(line, max(1, width-4)) {
			border := tuistyle.DiffFrameStyle.Render("  │ ")
			out = append(out, border+tuistyle.ThoughtBodyStyle.Render(wrapped))
		}
	}
	return out
}

func (c *ReasoningCell) RawLines() []string {
	if strings.TrimSpace(c.Content) == "" {
		return []string{c.summaryLabel()}
	}
	out := []string{c.summaryLabel()}
	for _, line := range strings.Split(c.Content, "\n") {
		out = append(out, sanitizeBubbleText(line))
	}
	return out
}

func (c *ReasoningCell) LineCount() int {
	return len(c.RawLines())
}

// Ensure interface satisfaction
var _ CollapsibleCell = (*ReasoningCell)(nil)
