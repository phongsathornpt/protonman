package pane

import (
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

type TodoSnapshot struct {
	Width  int
	Height int
	Offset int
	Items  []tododomain.Item
}

func TodoRows(snapshot TodoSnapshot) []string {
	completed, active, pending := TodoCounts(snapshot.Items)
	rows := []string{tuistyle.BrandStyle.Render(fmt.Sprintf("Tasks %d/%d · %d active · %d pending", completed, len(snapshot.Items), active, pending))}
	limit := TodoInspectionLimit(snapshot.Height)
	start := min(max(0, snapshot.Offset), len(snapshot.Items))
	end := min(len(snapshot.Items), start+limit)
	for _, item := range snapshot.Items[start:end] {
		glyph := tuistyle.GlyphTodoPending
		style := tuistyle.MutedStyle
		if item.Status == tododomain.StatusInProgress {
			glyph, style = tuistyle.GlyphTodoActive, tuistyle.BrandStyle
		}
		if item.Status == tododomain.StatusCompleted {
			glyph, style = tuistyle.GlyphToolSuccess, tuistyle.SuccessStyle
		}
		rows = append(rows, style.Render(glyph+tool.TruncateRunes(item.Text, max(12, snapshot.Width-8))))
		rows = append(rows, tuistyle.MutedStyle.Render("  id: "+strings.TrimSpace(item.ID)))
	}
	if len(snapshot.Items) > limit {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("%d-%d of %d · ↑/↓ scroll · esc close", start+1, end, len(snapshot.Items))))
	} else {
		rows = append(rows, tuistyle.MutedStyle.Render("esc close"))
	}
	return rows
}

func TodoInspectionLimit(height int) int { return max(1, min(8, (height-7)/2)) }

func TodoCounts(items []tododomain.Item) (completed, active, pending int) {
	for _, item := range items {
		switch item.Status {
		case tododomain.StatusCompleted:
			completed++
		case tododomain.StatusInProgress:
			active++
		case tododomain.StatusPending:
			pending++
		}
	}
	return completed, active, pending
}
