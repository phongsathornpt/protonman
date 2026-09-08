package pane

import (
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/textview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type PermissionSnapshot struct {
	Width        int
	Height       int
	Parked       bool
	Index        int
	Title        string
	Tone         Tone
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
	Tone   Tone
}

func PermissionView(snapshot PermissionSnapshot) PermissionRender {
	if snapshot.Parked {
		line := fmt.Sprintf("! Permission pending · %s · tab review · %s", snapshot.ToolName, snapshot.ShortcutHint)
		return PermissionRender{Inline: tuistyle.MutedStyle.Render(tool.TruncateRunes(line, max(1, snapshot.Width-2))), Tone: snapshot.Tone}
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
	case ToneUser:
		titleStyle = tuistyle.UserStyle
	case ToneError:
		titleStyle = tuistyle.ErrorStyle
	}
	if ModeForHeight(snapshot.Height) == LayoutTiny {
		contentWidth := max(8, snapshot.Width-8)
		selected := ""
		if len(snapshot.Options) > 0 {
			selected = snapshot.Options[index]
		}
		return PermissionRender{Rows: []string{
			titleStyle.Render(tool.TruncateRunes(snapshot.Title, contentWidth)),
			tuistyle.MutedStyle.Render(tool.TruncateRunes(snapshot.ToolName+" · "+snapshot.Detail, contentWidth)),
			tuistyle.BrandStyle.Render(tuistyle.GlyphPrompt + selected),
			tuistyle.MutedStyle.Render(snapshot.ShortcutHint),
			tuistyle.MutedStyle.Render("esc review"),
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
	if ModeForHeight(snapshot.Height) == LayoutCompact {
		maxDetailLines = 2
	}
	if len(detailLines) > maxDetailLines {
		omitted := len(detailLines) - maxDetailLines
		detailLines = append(detailLines[:maxDetailLines], fmt.Sprintf("... (%d more lines truncated)", omitted))
	}
	rows = append(rows, tuistyle.MutedStyle.Render(strings.Join(detailLines, "\n")), "")
	for i, option := range snapshot.Options {
		marker := "  "
		if i == index {
			marker = tuistyle.GlyphPrompt
			rows = append(rows, tuistyle.BrandStyle.Render(marker+option))
			continue
		}
		rows = append(rows, tuistyle.MutedStyle.Render(marker+option))
	}
	rows = append(rows, "")
	if snapshot.Parked {
		rows = append(rows, tuistyle.MutedStyle.Render("tab review approval   "+snapshot.ShortcutHint+"   pgup/pgdn scroll"))
	} else {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("j/k move   1-%d select   %s   esc review transcript", len(snapshot.Options), snapshot.ShortcutHint)))
	}
	if ModeForHeight(snapshot.Height) == LayoutCompact {
		rows = CompactRows(rows)
	}
	return PermissionRender{Rows: rows, Tone: snapshot.Tone}
}
