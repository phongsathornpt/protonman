package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"io"
	"strings"
)

const maxSlashRows = 6

const slashViewID = "slash"

type slashCommand = slashview.Command

var slashCatalog = slashview.Catalog(agent.ProfileList("|"))

type slashListItem struct{ command slashCommand }

func (i slashListItem) FilterValue() string {
	return i.command.Name + " " + strings.Join(i.command.Aliases, " ") + " " + i.command.Description
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
		v.picker = list.New(items, slashCommandDelegate{}, maxInt(20, ctx.width-4), maxSlashRows)
		v.picker.DisableQuitKeybindings()
		v.picker.SetFilteringEnabled(false)
		v.picker.SetShowTitle(false)
		v.picker.SetShowStatusBar(false)
		// Slash completion owns navigation through list.Update; help lives in the shared composer footer.
		v.picker.SetShowPagination(false)
		v.picker.SetShowHelp(false)
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
	rows := []string{brandStyle.Render("Commands"), ""}
	rows = append(rows, v.commandRows(ctx)...)
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
		if description == "" || available-nameWidth < 8 {
			rows = append(rows, prefix+nameStyle.Render(truncateWithEllipsis(name, available)))
			continue
		}
		description = truncateWithEllipsis(description, maxInt(1, available-nameWidth-2))
		rows = append(rows, prefix+nameStyle.Render(name)+"  "+mutedStyle.Render(description))
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
	selected := v.matches[index]
	name := "/" + selected.Name
	status := fmt.Sprintf("%s · %d/%d", name, index+1, len(v.matches))
	if remaining := len(v.matches) - minInt(maxSlashRows, len(v.matches)); remaining > 0 {
		status += fmt.Sprintf(" · %d more", remaining)
	}
	return paneRightStatus(width, status)
}

func (v *slashPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.sync(ctx)
	switch message.String() {
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	case "tab":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionAcceptSlash}}
	case "enter":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionAcceptSlash, runSlash: true}}
	case "esc":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: slashViewID}}
	default:
		return paneKeyResult{}
	}
}
