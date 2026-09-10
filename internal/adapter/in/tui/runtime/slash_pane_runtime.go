package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
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

type slashPaneView struct {
	picker  list.Model
	ready   bool
	matches []slashCommand
}

func (*slashPaneView) ID() string             { return slashViewID }
func (*slashPaneView) ReplacesComposer() bool { return false }

func (v *slashPaneView) sync(ctx paneRenderContext) {
	matches := ctx.slashMatches
	v.matches = append(v.matches[:0], matches...)
	items := make([]list.Item, 0, len(matches))
	for _, command := range matches {
		items = append(items, slashListItem{command: command})
	}
	if !v.ready {
		delegate := list.NewDefaultDelegate()
		delegate.SetSpacing(0)
		v.picker = list.New(items, delegate, maxInt(20, ctx.width-4), maxInt(4, minInt(12, ctx.height/2)))
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
	v.picker.SetSize(maxInt(20, ctx.width-4), maxInt(4, minInt(12, ctx.height/2)))
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = layoutModeForHeight(ctx.height) == layoutNormal
	v.picker.SetDelegate(delegate)
	rows := strings.Split(v.picker.View(), "\n")
	if remaining := len(v.matches) - minInt(maxSlashRows, len(v.matches)); remaining > 0 {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("↓ %d more", remaining)))
	}
	if layoutModeForHeight(ctx.height) != layoutTiny {
		rows = append(rows, mutedStyle.Render("↑↓ navigate · enter select · tab complete · esc close"))
	}
	return strings.Join(rows, "\n")
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
