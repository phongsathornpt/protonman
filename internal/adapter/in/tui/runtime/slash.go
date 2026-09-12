package runtime

import (
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

const maxSlashRows = 6

const slashViewID = "slash"

type slashCommand = slashview.Command

var slashCatalog = slashview.Catalog()

type slashListItem struct{ command slashCommand }

func (i slashListItem) FilterValue() string { return i.command.FilterValue() }
func (i slashListItem) Title() string       { return i.command.Title() }
func (i slashListItem) Description() string { return i.command.DisplayDescription() }

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
		v.picker = paneutil.NewMinimalList(items, slashCommandDelegate{}, maxInt(20, ctx.width-4), maxSlashRows)
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

// Render reads the last synced picker snapshot; callers must sync through the
// Update boundary (syncSlashView) before rendering so View stays side-effect free.
func (v *slashPaneView) Render(ctx paneRenderContext) string {
	if !v.ready || len(v.matches) == 0 {
		return ""
	}
	visibleRows := minInt(maxSlashRows, len(v.matches))
	v.picker.SetSize(maxInt(20, ctx.width-4), maxInt(1, visibleRows))
	rows := v.commandRows(ctx)
	if layoutModeForHeight(ctx.height) != layoutTiny {
		width := maxInt(1, ctx.width-6)
		rows = append(rows, paneHelpStatusLine(width, slashPickerHelp(width), v.selectionStatusText()))
	} else if status := v.selectionStatus(ctx.width); status != "" {
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

func (v *slashPaneView) selectionStatusText() string {
	if len(v.matches) == 0 {
		return ""
	}
	index := maxInt(0, minInt(v.picker.GlobalIndex(), len(v.matches)-1))
	return fmt.Sprintf("%d/%d", index+1, len(v.matches))
}

func (v *slashPaneView) selectionStatus(width int) string {
	return paneRightStatus(width, v.selectionStatusText())
}

func (v *slashPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.sync(ctx)
	switch {
	// The slash pane renders below the composer and shares the draft with it,
	// so it may only claim navigation keys that cannot be typed. Matching the
	// modal paneutil.Keys.Nav here would swallow "j"/"k"/"g"/"G" from the
	// command being typed (for example "/goal" becoming "/oal").
	case key.Matches(message, paneutil.Keys.CompletionNav):
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	case key.Matches(message, paneutil.Keys.Tab):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionAcceptSlash}}
	case key.Matches(message, paneutil.Keys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionAcceptSlash, runSlash: true}}
	case key.Matches(message, paneutil.Keys.CompletionClose):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: slashViewID}}
	default:
		return paneKeyResult{}
	}
}

func isCommandLine(line string) bool {
	return slashview.IsCommandLine(line)
}

func splitCommand(line string) (string, string, []string) {
	return slashview.SplitCommand(line)
}

func parseCommand(line string) slashview.ParsedCommand {
	return slashview.ParseCommand(line)
}

func canonicalSlashName(name string) string {
	return slashview.CanonicalName(name)
}

func fuzzyContains(target, query string) bool {
	return slashview.FuzzyContains(target, query)
}

type slashContext = slashview.Context

const (
	slashKindCommand = slashview.ContextCommand
	slashKindSkill   = slashview.ContextSkill
	slashKindLow     = slashview.ContextLowConcurrency
)

func (m bubbleModel) parseSlashContext() (slashContext, bool) {
	if m.panes.bottom == nil || m.panes.bottom.bashMode() || m.panes.bottom.has(permissionViewID) || m.panes.bottom.has(skillsViewID) {
		return slashContext{}, false
	}
	prompt := m.panes.bottom.prompt()
	if prompt == nil {
		return slashContext{}, false
	}
	return slashview.ParseContext(prompt.Value())
}

func (m bubbleModel) slashQuery() (prefix string, query string, ok bool) {
	context, ok := m.parseSlashContext()
	if !ok {
		return "", "", false
	}
	return context.Prefix, context.Query, true
}

func (m bubbleModel) slashMatches() []slashCommand {
	context, ok := m.parseSlashContext()
	if !ok {
		return nil
	}
	if context.Kind != slashKindSkill {
		return slashview.Matches(context, slashCatalog, nil)
	}
	if m.skills == nil {
		return nil
	}
	skills := m.skills.List()
	items := make([]slashview.Skill, 0, len(skills))
	for _, skill := range skills {
		items = append(items, slashview.Skill{Name: skill.Name, Description: skill.Description, Scope: string(skill.Scope), Active: m.skills.IsActivated(skill.Name)})
	}
	return slashview.Matches(context, slashCatalog, items)
}

func (m *bubbleModel) slashState() *slashPaneView {
	if m.panes.bottom == nil {
		return nil
	}
	view, _ := m.panes.bottom.find(slashViewID).(*slashPaneView)
	return view
}

func (m *bubbleModel) syncSlashView() {
	if m.panes.bottom == nil {
		return
	}
	matches := m.slashMatches()
	if len(matches) == 0 {
		m.panes.bottom.remove(slashViewID)
		return
	}
	view := m.slashState()
	if view == nil {
		view = &slashPaneView{}
		m.panes.bottom.push(view)
	}
	view.sync(newPaneRenderContext(m))
}

func (m bubbleModel) slashOpen() bool {
	return len(m.slashMatches()) > 0
}

func (m *bubbleModel) clampSlashIndex() {
	m.syncSlashView()
	if view := m.slashState(); view != nil {
		view.sync(newPaneRenderContext(m))
	}
}

func (m *bubbleModel) acceptSlash(run bool) (applied bool, command tea.Cmd) {
	m.syncSlashView()
	view := m.slashState()
	matches := m.slashMatches()
	if view == nil || len(matches) == 0 {
		return false, nil
	}
	m.clampSlashIndex()
	view = m.slashState()
	if view == nil || len(view.matches) == 0 {
		return false, nil
	}
	selected := view.matches[view.picker.Index()]
	context, _ := m.parseSlashContext()
	prompt := m.panes.bottom.prompt()
	var insertion string
	if context.Kind == slashKindSkill || context.Kind == slashKindLow {
		insertion = context.Lead + selected.Name
	} else {
		insertion = context.Prefix + selected.Name
		if selected.Argument != slashview.ArgumentNone {
			insertion += " "
			prompt.SetValue(insertion)
			prompt.CursorEnd()
			m.panes.bottom.remove(slashViewID)
			return true, nil
		}
	}
	if !run {
		prompt.SetValue(insertion)
		prompt.CursorEnd()
		m.panes.bottom.remove(slashViewID)
		return true, nil
	}
	m.resetPrompt()
	m.panes.bottom.remove(slashViewID)
	return true, m.dispatch(insertion)
}

func truncateWithEllipsis(s string, maxLen int) string {
	return textview.TruncateEllipsis(s, maxLen)
}
