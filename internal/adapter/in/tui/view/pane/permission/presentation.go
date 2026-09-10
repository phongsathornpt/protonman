package permission

import (
	"fmt"
	"strings"

	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

type PermissionSnapshot struct {
	Width        int
	Height       int
	Parked       bool
	Index        int
	Title        string
	Tone         panecommon.Tone
	ToolName     string
	ToolKind     string
	Detail       string
	DetailExtras []string
	Options      []string
	ShortcutHint string
}

type PermissionRender struct {
	Inline string
	Rows   []string
	Tone   panecommon.Tone
}

func PermissionView(snapshot PermissionSnapshot) PermissionRender {
	if snapshot.Parked {
		line := fmt.Sprintf("! Permission pending · %s · tab review · %s", snapshot.ToolName, snapshot.ShortcutHint)
		return PermissionRender{Inline: tuistyle.MutedStyle.Render(textview.TruncateEllipsis(line, max(1, snapshot.Width-2))), Tone: snapshot.Tone}
	}
	index := snapshot.Index
	if index < 0 {
		index = 0
	}
	if len(snapshot.Options) > 0 && index >= len(snapshot.Options) {
		index = len(snapshot.Options) - 1
	}
	titleStyle := tuistyle.WarningStyle
	switch snapshot.Tone {
	case panecommon.ToneUser:
		titleStyle = tuistyle.UserStyle
	case panecommon.ToneError:
		titleStyle = tuistyle.ErrorStyle
	}
	if panecommon.ModeForHeight(snapshot.Height) == panecommon.LayoutTiny {
		contentWidth := max(8, snapshot.Width-8)
		selected := ""
		if len(snapshot.Options) > 0 {
			selected = snapshot.Options[index]
		}
		return PermissionRender{Rows: []string{
			titleStyle.Render(textview.TruncateEllipsis(snapshot.Title, contentWidth)),
			tuistyle.MutedStyle.Render(textview.TruncateEllipsis(snapshot.ToolName+" · "+snapshot.Detail, contentWidth)),
			tuistyle.BrandStyle.Render(tuistyle.GlyphPrompt + selected),
		}, Tone: snapshot.Tone}
	}
	maxWidth := max(1, snapshot.Width-8)
	rows := make([]string, 0, 8)
	rows = append(rows, titleStyle.Render(snapshot.Title))
	rows = append(rows, fmt.Sprintf("%s (%s)", snapshot.ToolName, snapshot.ToolKind))
	detailLines := textview.WrapLines("Target: "+snapshot.Detail, max(1, maxWidth-6))
	for _, extra := range snapshot.DetailExtras {
		detailLines = append(detailLines, textview.WrapLines(extra, max(1, maxWidth-6))...)
	}
	maxDetailLines := 6
	if panecommon.ModeForHeight(snapshot.Height) == panecommon.LayoutCompact {
		maxDetailLines = 2
	}
	if len(detailLines) > maxDetailLines {
		omitted := len(detailLines) - maxDetailLines
		detailLines = append(detailLines[:maxDetailLines], fmt.Sprintf("... (%d more lines truncated)", omitted))
	}
	rows = append(rows, tuistyle.MutedStyle.Render(strings.Join(detailLines, "\n")))
	for i, option := range snapshot.Options {
		marker := "  "
		if i == index {
			marker = tuistyle.GlyphPrompt
			rows = append(rows, tuistyle.BrandStyle.Render(marker+option))
			continue
		}
		rows = append(rows, tuistyle.MutedStyle.Render(marker+option))
	}
	if panecommon.ModeForHeight(snapshot.Height) == panecommon.LayoutCompact {
		rows = panecommon.CompactRows(rows)
	}
	return PermissionRender{Rows: rows, Tone: snapshot.Tone}
}
