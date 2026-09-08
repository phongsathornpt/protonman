package pane

import (
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type ProviderItem struct {
	DisplayName string
	BaseURL     string
	Description string
	Configured  bool
	Active      bool
	Free        bool
	Custom      bool
}

type ProviderSnapshot struct {
	Width         int
	Height        int
	ContentWidth  int
	Index         int
	Offset        int
	ActiveName    string
	DeleteConfirm bool
	Items         []ProviderItem
}

func ProviderRows(snapshot ProviderSnapshot) (rows []string, warning bool) {
	if len(snapshot.Items) == 0 {
		return []string{
			tuistyle.BrandStyle.Render("✓ Model Providers"), "",
			tuistyle.MutedStyle.Render("No providers or presets available."), "",
			tuistyle.MutedStyle.Render("a add provider · esc close"),
		}, false
	}
	visibleRows := PickerVisibleRows(snapshot.Height, 5)
	index, offset, visibleEnd := NormalizedWindow(snapshot.Index, snapshot.Offset, len(snapshot.Items), visibleRows)
	if snapshot.DeleteConfirm {
		item := snapshot.Items[index]
		if item.Configured {
			rows := []string{
				tuistyle.WarningStyle.Render("Remove Provider?"), "",
				fmt.Sprintf("  %s", item.DisplayName),
				tuistyle.MutedStyle.Render("  " + item.BaseURL),
			}
			if item.Active {
				rows = append(rows, tuistyle.WarningStyle.Render("  This is the active provider."))
				rows = append(rows, tuistyle.MutedStyle.Render("  Protonman will select another saved provider."))
			}
			return append(rows, "", tuistyle.MutedStyle.Render("enter remove permanently · esc cancel")), true
		}
	}
	activeName := strings.TrimSpace(snapshot.ActiveName)
	if activeName == "" {
		activeName = "none"
	}
	contentWidth := max(1, snapshot.ContentWidth)
	title := fmt.Sprintf("Providers · active: %s · %d/%d", activeName, index+1, len(snapshot.Items))
	title = tool.TruncateRunes(title, contentWidth)
	rows = make([]string, 0, (visibleEnd-offset)*2+6)
	rows = append(rows, tuistyle.BrandStyle.Render(title), "")
	if offset > 0 {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("  ↑ %d more", offset)))
	}
	itemWidth := max(8, contentWidth-2)
	showDetails := ModeForHeight(snapshot.Height) == LayoutNormal
	for i, item := range snapshot.Items[offset:visibleEnd] {
		idx := offset + i
		focus := "  "
		if idx == index {
			focus = "❯ "
		}
		active := " "
		if item.Active {
			active = "✓"
		}
		status := "setup"
		switch {
		case item.Active:
			status = "active"
		case item.Configured:
			status = "saved"
		case item.Custom:
			status = "custom"
		}
		line := fmt.Sprintf("%s%s %s · %s", focus, active, item.DisplayName, status)
		if item.Free {
			line += " · free"
		}
		line = tool.TruncateRunes(line, itemWidth)
		switch {
		case idx == index:
			rows = append(rows, tuistyle.BrandStyle.Render(line))
		case item.Active:
			rows = append(rows, tuistyle.SuccessStyle.Render(line))
		default:
			rows = append(rows, tuistyle.MutedStyle.Render(line))
		}
		if showDetails {
			detail := item.BaseURL
			if item.Custom || detail == "" {
				detail = item.Description
			}
			if detail != "" {
				rows = append(rows, tuistyle.MutedStyle.Render("    "+tool.TruncateRunes(detail, max(4, itemWidth-4))))
			}
		}
	}
	if visibleEnd < len(snapshot.Items) {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("  ↓ %d more", len(snapshot.Items)-visibleEnd)))
	}
	footer := "↑/↓ move · enter activate/setup · e edit · m models · d remove · esc"
	if snapshot.Height <= 20 {
		footer = "↑/↓ move · enter activate/setup · e edit · esc"
		if snapshot.Width <= 30 {
			footer = "↑/↓ · enter · esc"
		}
	}
	return append(rows, "", tuistyle.MutedStyle.Render(footer)), false
}
