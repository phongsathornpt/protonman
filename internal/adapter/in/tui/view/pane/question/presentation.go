package question

import (
	"fmt"
	"strings"

	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

// QuestionSnapshot captures the state of the question prompt for rendering.
type QuestionSnapshot struct {
	Width       int
	Height      int
	Question    string
	Options     []string
	Multiple    bool
	Index       int
	Selected    map[int]bool
	WriteInMode bool
	WriteInText string
	Tone        panecommon.Tone
}

// QuestionRender represents the rendered pane content.
type QuestionRender struct {
	Rows []string
	Tone panecommon.Tone
}

// QuestionView renders the question modal rows.
func QuestionView(snapshot QuestionSnapshot) QuestionRender {
	contentWidth := max(1, snapshot.Width-8)
	titleStyle := tuistyle.UserStyle
	if snapshot.Tone == panecommon.ToneAssistant {
		titleStyle = tuistyle.AssistantStyle
	}

	mode := panecommon.ModeForSize(snapshot.Width, snapshot.Height)
	rows := make([]string, 0, 16)

	// Title
	rows = append(rows, titleStyle.Render("Question from Assistant"))

	// Question text
	qLines := textview.WrapLines(snapshot.Question, contentWidth)
	maxQLines := 4
	if mode == panecommon.LayoutCompact {
		maxQLines = 2
	}
	if len(qLines) > maxQLines {
		qLines = append(qLines[:maxQLines], textview.TruncateEllipsis("...", contentWidth))
	}
	for _, l := range qLines {
		rows = append(rows, tuistyle.BodyStyle.Render(l))
	}
	rows = append(rows, "")

	hasOptions := len(snapshot.Options) > 0

	if snapshot.WriteInMode || !hasOptions {
		// Render text input prompt
		promptLabel := "Your answer: "
		if hasOptions {
			promptLabel = "Custom answer: "
		}
		inputText := snapshot.WriteInText
		cursor := "_"
		rows = append(rows, tuistyle.GlyphPrompt+" "+titleStyle.Render(promptLabel)+inputText+tuistyle.FocusStyle.Render(cursor))
		rows = append(rows, "")
		help := "enter: submit · esc: "
		if hasOptions {
			help += "cancel write-in"
		} else {
			help += "decline"
		}
		rows = append(rows, tuistyle.MutedStyle.Render(help))
		return QuestionRender{Rows: rows, Tone: snapshot.Tone}
	}

	// Render Options
	for i, opt := range snapshot.Options {
		isCursor := i == snapshot.Index
		cursor := "  "
		if isCursor {
			cursor = tuistyle.GlyphPrompt + " "
		}

		check := ""
		if snapshot.Multiple {
			if snapshot.Selected[i] {
				check = "[x] "
			} else {
				check = "[ ] "
			}
		}

		shortcut := ""
		if i < 9 {
			shortcut = fmt.Sprintf("[%d] ", i+1)
		}

		line := textview.TruncateEllipsis(cursor+shortcut+check+opt, contentWidth)
		if isCursor {
			rows = append(rows, tuistyle.SelectionStyle.Render(line))
		} else {
			rows = append(rows, tuistyle.MutedStyle.Render(line))
		}
	}

	// Write-in option row
	writeInIdx := len(snapshot.Options)
	isWriteInCursor := snapshot.Index == writeInIdx
	wCursor := "  "
	if isWriteInCursor {
		wCursor = tuistyle.GlyphPrompt + " "
	}
	wLine := textview.TruncateEllipsis(wCursor+"[W] Write custom response...", contentWidth)
	if isWriteInCursor {
		rows = append(rows, tuistyle.SelectionStyle.Render(wLine))
	} else {
		rows = append(rows, tuistyle.MutedStyle.Render(wLine))
	}

	rows = append(rows, "")
	helpParts := []string{"↑/↓: move", "1-N: pick"}
	if snapshot.Multiple {
		helpParts = append(helpParts, "space: toggle")
	}
	helpParts = append(helpParts, "w: write-in", "enter: confirm", "esc: decline")
	rows = append(rows, tuistyle.MutedStyle.Render(strings.Join(helpParts, " · ")))

	return QuestionRender{Rows: rows, Tone: snapshot.Tone}
}
