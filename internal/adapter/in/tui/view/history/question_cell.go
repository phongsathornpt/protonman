package history

import (
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

// QuestionCell renders an interactive question and the user's resolution in the transcript.
type QuestionCell struct {
	Question string
	Answer   string
	Declined bool
}

func (QuestionCell) Kind() HistoryCellKind { return HistoryCellTool }

func (c QuestionCell) RenderWidth(width int) []string {
	contentWidth := max(1, width-4)
	rows := make([]string, 0, 8)

	// Question text
	header := tuistyle.InfoStyle.Render("? ") + tuistyle.BodyStyle.Render(c.Question)
	wrappedQ := textview.WrapLines(header, contentWidth)
	rows = append(rows, wrappedQ...)

	// Answer badge
	if c.Declined {
		declined := tuistyle.MutedStyle.Render("  [Declined] " + c.Answer)
		rows = append(rows, declined)
	} else if c.Answer != "" {
		ans := tuistyle.SuccessStyle.Render("  ✔ ") + tuistyle.UserStyle.Render(c.Answer)
		wrappedA := textview.WrapLines(ans, contentWidth)
		rows = append(rows, wrappedA...)
	}

	return rows
}

func (c QuestionCell) RawLines() []string {
	lines := []string{"Question: " + c.Question}
	if c.Declined {
		lines = append(lines, "Answer: [Declined] "+c.Answer)
	} else if c.Answer != "" {
		lines = append(lines, "Answer: "+c.Answer)
	}
	return lines
}

func (c QuestionCell) LineCount() int {
	count := 1
	if c.Answer != "" {
		count++
	}
	return count
}
