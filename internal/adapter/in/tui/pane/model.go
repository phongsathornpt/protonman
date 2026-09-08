package pane

import (
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type ModelItem struct {
	ID      string
	Label   string
	Free    bool
	Current bool
	Details string
}

type ModelSnapshot struct {
	Width         int
	Height        int
	Index         int
	Offset        int
	ProviderName  string
	ProviderCount int
	Models        []ModelItem
	Filter        string
	Filtering     bool
	Loading       bool
	ErrorText     string
}

func ModelRows(snapshot ModelSnapshot) []string {
	maxWidth := max(1, snapshot.Width-4)
	if snapshot.Loading {
		return []string{
			tuistyle.BrandStyle.Render("Select Model · " + snapshot.ProviderName), "",
			tuistyle.MutedStyle.Render("Loading models..."), "",
			tuistyle.MutedStyle.Render("esc close"),
		}
	}
	if snapshot.ErrorText != "" {
		return []string{
			tuistyle.BrandStyle.Render("Select Model · " + snapshot.ProviderName), "",
			tuistyle.ErrorStyle.Render("Failed to load models"),
			tuistyle.MutedStyle.Render(tool.TruncateRunes(snapshot.ErrorText, max(8, maxWidth-4))), "",
			tuistyle.MutedStyle.Render("r retry · p providers · esc close"),
		}
	}
	if len(snapshot.Models) == 0 {
		return []string{
			tuistyle.BrandStyle.Render("✓ Select Model"), "",
			tuistyle.MutedStyle.Render("No models available for the selected provider."), "",
			tuistyle.MutedStyle.Render("a add provider credentials · esc close"),
		}
	}
	visibleRows := PickerVisibleRows(snapshot.Height, 6)
	index, offset, visibleEnd := NormalizedWindow(snapshot.Index, snapshot.Offset, len(snapshot.Models), visibleRows)
	visible := snapshot.Models[offset:visibleEnd]
	title := fmt.Sprintf("Select Model · %s · %d/%d", snapshot.ProviderName, index+1, len(snapshot.Models))
	if snapshot.ProviderCount > 1 {
		title += " · tab provider"
	}
	rows := make([]string, 0, len(visible)*2+8)
	rows = append(rows, tuistyle.BrandStyle.Render(title))
	if snapshot.Filtering || strings.TrimSpace(snapshot.Filter) != "" {
		search := "Search: " + snapshot.Filter
		if snapshot.Filtering {
			search += "█"
		}
		rows = append(rows, tuistyle.MutedStyle.Render(tool.TruncateRunes(search, maxWidth-2)))
	}
	rows = append(rows, "")
	if offset > 0 {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("  ↑ %d more", offset)))
	}
	contentWidth := max(8, maxWidth-6)
	showDetails := ModeForHeight(snapshot.Height) == LayoutNormal
	for i, item := range visible {
		idx := offset + i
		focus := "  "
		if idx == index {
			focus = "❯ "
		}
		active := " "
		if item.Current {
			active = "✓"
		}
		label := strings.TrimSpace(item.Label)
		if label == "" {
			label = item.ID
		}
		line := fmt.Sprintf("%s%s %s", focus, active, label)
		if item.Free {
			line += " · FREE"
		}
		line = tool.TruncateRunes(line, contentWidth)
		switch {
		case idx == index:
			rows = append(rows, tuistyle.BrandStyle.Render(line))
		case item.Current:
			rows = append(rows, tuistyle.SuccessStyle.Render(line))
		default:
			rows = append(rows, tuistyle.MutedStyle.Render(line))
		}
		if showDetails && strings.TrimSpace(item.Details) != "" {
			rows = append(rows, tuistyle.MutedStyle.Render("    "+tool.TruncateRunes(item.Details, max(4, contentWidth-4))))
		}
	}
	if visibleEnd < len(snapshot.Models) {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("  ↓ %d more", len(snapshot.Models)-visibleEnd)))
	}
	footer := "↑/↓ move · enter select · / search · ctrl+u clear · p providers · esc close"
	if ModeForHeight(snapshot.Height) == LayoutTiny {
		footer = "↑/↓ · enter · esc"
		rows = CompactRows(rows)
	}
	return append(rows, "", tuistyle.MutedStyle.Render(footer))
}
