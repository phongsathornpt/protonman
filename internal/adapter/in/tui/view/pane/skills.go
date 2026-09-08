package pane

import (
	"fmt"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

type SkillItem struct {
	Name   string
	Active bool
}

type SkillsSnapshot struct {
	Width             int
	Height            int
	Index             int
	Offset            int
	Items             []SkillItem
	UserSkillsDisplay string
}

func SkillsRows(snapshot SkillsSnapshot) []string {
	if len(snapshot.Items) == 0 {
		return []string{
			"No agent skills discovered.",
			"",
			fmt.Sprintf("Place skills in %s or .protonman/skills/.", snapshot.UserSkillsDisplay),
			"",
			"esc close",
		}
	}
	visibleRows := PickerVisibleRows(snapshot.Height, 6)
	index, offset, visibleEnd := NormalizedWindow(snapshot.Index, snapshot.Offset, len(snapshot.Items), visibleRows)
	visible := snapshot.Items[offset:visibleEnd]
	maxWidth := max(1, snapshot.Width-8)
	activeCount := 0
	for _, item := range snapshot.Items {
		if item.Active {
			activeCount++
		}
	}
	rows := make([]string, 0, len(visible)+6)
	rows = append(rows, tuistyle.BrandStyle.Render(fmt.Sprintf("Agent Skills (%d/%d active · item %d of %d)", activeCount, len(snapshot.Items), index+1, len(snapshot.Items))), "")
	if offset > 0 {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("  ▲ %d more above", offset)))
	}
	for i, item := range visible {
		idx := offset + i
		current := idx == index
		cursor := "  "
		if current {
			cursor = tuistyle.GlyphPrompt
		}
		box := tuistyle.MutedStyle.Render("[ ]")
		if item.Active {
			box = tuistyle.SuccessStyle.Render("[x]")
		}
		name := tuistyle.AssistantStyle.Render(item.Name)
		if current {
			name = tuistyle.BrandStyle.Bold(true).Render(item.Name)
		} else if item.Active {
			name = tuistyle.AssistantStyle.Bold(true).Render(item.Name)
		}
		rows = append(rows, textview.WrapWords(fmt.Sprintf("%s%s %s", cursor, box, name), maxWidth))
	}
	if visibleEnd < len(snapshot.Items) {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("  ▼ %d more below", len(snapshot.Items)-visibleEnd)))
	}
	footer := "j/k move · space toggle · pgup/pgdn · esc/enter close"
	if ModeForHeight(snapshot.Height) == LayoutTiny {
		footer = "↑/↓ · space · esc"
		rows = CompactRows(rows)
	}
	return append(rows, "", tuistyle.MutedStyle.Render(footer))
}
