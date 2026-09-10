package runtime

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	"io"
	"strings"
)

const maxSlashRows = 6

const slashViewID = "slash"

type slashCommand = slashview.Command

var slashCatalog = slashview.Catalog()

type slashListItem struct{ command slashCommand }

func (i slashListItem) FilterValue() string {
	return i.command.Name + " " + i.command.Description
}
func (i slashListItem) Title() string {
	prefix := i.command.PrefixTag
	if prefix != "" {
		return prefix + " " + i.command.Name
	}
	return "/" + i.command.Name
}
func (i slashListItem) Description() string {
	if i.command.Scope != "" {
		return i.command.Description + " · " + i.command.Scope
	}
	return i.command.Description
}

type slashCommandDelegate struct{}

func (slashCommandDelegate) Height() int                         { return 1 }
func (slashCommandDelegate) Spacing() int                        { return 0 }
func (slashCommandDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (slashCommandDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(slashListItem)
	if !ok {
		return
	}
	prefix := "  "
	nameStyle := bodyStyle
	if index == m.Index() {
		prefix = glyphPrompt
		nameStyle = brandStyle
	}
	name := entry.Title()
	description := entry.Description()
	available := maxInt(1, m.Width()-2)
	nameWidth := len([]rune(name))
	if description == "" || available-nameWidth < 8 {
		_, _ = fmt.Fprint(w, prefix+nameStyle.Render(truncateWithEllipsis(name, available)))
		return
	}
	description = truncateWithEllipsis(description, maxInt(1, available-nameWidth-2))
	_, _ = fmt.Fprint(w, prefix+nameStyle.Render(name)+"  "+mutedStyle.Render(description))
}

type slashPaneView struct {
	picker  list.Model
	ready   bool
	matches []slashCommand
}

func (*slashPaneView) ID() string                             { return slashViewID }
func (*slashPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *slashPaneView) sync(ctx paneRenderContext) {
	matches := ctx.slashMatches
	v.matches = append(v.matches[:0], matches...)
	items := make([]list.Item, 0, len(matches))
	for _, command := range matches {
		items = append(items, slashListItem{command: command})
	}
	if !v.ready {
		v.picker = newMinimalList(items, slashCommandDelegate{}, maxInt(20, ctx.width-4), maxSlashRows)
		v.picker.SetFilteringEnabled(false)
		// Slash completion owns navigation through list.Update; help lives in the shared composer footer.
		v.picker.InfiniteScrolling = true
		v.ready = true
	} else {
		_ = v.picker.SetItems(items)
	}
	if len(matches) == 0 {
		return
	}
	selected := maxInt(0, minInt(v.picker.Index(), len(matches)-1))
	v.picker.Select(selected)
}

func (v *slashPaneView) Render(ctx paneRenderContext) string {
	v.sync(ctx)
	if len(v.matches) == 0 {
		return ""
	}
	visibleRows := minInt(maxSlashRows, len(v.matches))
	v.picker.SetSize(maxInt(20, ctx.width-4), maxInt(1, visibleRows))
	rows := v.commandRows(ctx)
	if layoutModeForHeight(ctx.height) != layoutTiny {
		rows = append(rows, "", slashPickerHelp(ctx.width))
	}
	if status := v.selectionStatus(ctx.width); status != "" {
		rows = append(rows, status)
	}
	return strings.Join(rows, "\n")
}

func (v *slashPaneView) commandRows(ctx paneRenderContext) []string {
	items := v.picker.VisibleItems()
	if len(items) == 0 {
		return nil
	}
	start, end := paneWindow(len(items), v.picker.Index(), maxSlashRows, layoutModeForHeight(ctx.height))
	rows := make([]string, 0, end-start)
	available := maxInt(1, ctx.width-6)
	nameColumnWidth := 0
	for i := start; i < end; i++ {
		entry, ok := items[i].(slashListItem)
		if !ok {
			continue
		}
		nameColumnWidth = maxInt(nameColumnWidth, len([]rune(entry.Title())))
	}
	for i := start; i < end; i++ {
		entry, ok := items[i].(slashListItem)
		if !ok {
			continue
		}
		prefix := "  "
		nameStyle := bodyStyle
		if i == v.picker.Index() {
			prefix = glyphPrompt
			nameStyle = brandStyle
		}
		name := entry.Title()
		description := entry.Description()
		nameWidth := len([]rune(name))
		if description == "" || available-nameColumnWidth < 8 {
			rows = append(rows, prefix+nameStyle.Render(truncateWithEllipsis(name, available)))
			continue
		}
		descriptionWidth := maxInt(1, available-nameColumnWidth-2)
		description = truncateWithEllipsis(description, descriptionWidth)
		gap := strings.Repeat(" ", maxInt(2, nameColumnWidth-nameWidth+2))
		rows = append(rows, prefix+nameStyle.Render(name)+gap+mutedStyle.Render(description))
	}
	return rows
}

func slashPickerHelp(width int) string {
	return paneKeyboardHelp(width, "↑/↓", "Navigate", "enter", "Select", "tab", "Complete", "esc", "Go Back")
}

func (v *slashPaneView) selectionStatus(width int) string {
	if len(v.matches) == 0 {
		return ""
	}
	index := maxInt(0, minInt(v.picker.GlobalIndex(), len(v.matches)-1))
	status := fmt.Sprintf("%d/%d", index+1, len(v.matches))
	return paneRightStatus(width, status)
}

func (v *slashPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.sync(ctx)
	switch {
	case key.Matches(message, paneKeys.Nav):
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	case key.Matches(message, paneKeys.Tab):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionAcceptSlash}}
	case key.Matches(message, paneKeys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionAcceptSlash, runSlash: true}}
	case key.Matches(message, paneKeys.Close):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: slashViewID}}
	default:
		return paneKeyResult{}
	}
}
