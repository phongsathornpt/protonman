package runtime

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/mentionview"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/base/glob"
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
		prefix = tuistyle.SelectionStyle.Render(glyphPrompt)
		nameStyle = tuistyle.SelectionStyle
	}
	name := entry.Title()
	description := entry.Description()
	available := max(1, m.Width()-2)
	nameWidth := len([]rune(name))
	if description == "" || available-nameWidth < 8 {
		_, _ = fmt.Fprint(w, prefix+nameStyle.Render(truncateWithEllipsis(name, available)))
		return
	}
	description = truncateWithEllipsis(description, max(1, available-nameWidth-2))
	_, _ = fmt.Fprint(w, prefix+nameStyle.Render(name)+"  "+mutedStyle.Render(description))
}

// slashPickerWidth returns the list width for the slash completion picker. The
// picker sits below the composer and uses a 4-cell horizontal inset.
func slashPickerWidth(width int) int {
	return max(1, width-4)
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
		v.picker = paneutil.NewMinimalList(items, slashCommandDelegate{}, slashPickerWidth(ctx.width), maxSlashRows)
		v.picker.SetFilteringEnabled(false)
		// Slash completion owns navigation through list.Update; help lives in the shared composer footer.
		v.picker.InfiniteScrolling = true
		v.ready = true
	} else {
		_ = v.picker.SetItems(items)
	}
	visibleRows := min(maxSlashRows, len(matches))
	v.picker.SetSize(slashPickerWidth(ctx.width), max(1, visibleRows))
	if len(matches) == 0 {
		return
	}
	selected := max(0, min(v.picker.Index(), len(matches)-1))
	v.picker.Select(selected)
}

// Render reads the last synced picker snapshot; callers must sync through the
// Update boundary (syncSlashView) before rendering so View stays side-effect free.
func (v *slashPaneView) Render(ctx paneRenderContext) string {
	if !v.ready || len(v.matches) == 0 {
		return ""
	}
	rows := v.commandRows(ctx)
	if layoutModeForHeight(ctx.height) != layoutTiny {
		width := panecommon.PaneHelpWidth(ctx.width)
		statusText := v.selectionStatusText()
		helpWidth := max(1, width-ansi.StringWidth(statusText)-1)
		rows = append(rows, paneHelpStatusLine(width, slashPickerHelp(helpWidth), statusText))
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
	available := panecommon.PaneHelpWidth(ctx.width)
	nameColumnWidth := 0
	for i := start; i < end; i++ {
		entry, ok := items[i].(slashListItem)
		if !ok {
			continue
		}
		nameColumnWidth = max(nameColumnWidth, len([]rune(entry.Title())))
	}
	for i := start; i < end; i++ {
		entry, ok := items[i].(slashListItem)
		if !ok {
			continue
		}
		prefix := "  "
		nameStyle := bodyStyle
		if i == v.picker.Index() {
			prefix = tuistyle.SelectionStyle.Render(glyphPrompt)
			nameStyle = tuistyle.SelectionStyle
		}
		name := entry.Title()
		description := entry.Description()
		nameWidth := len([]rune(name))
		if description == "" || available-nameColumnWidth < 8 {
			rows = append(rows, prefix+nameStyle.Render(truncateWithEllipsis(name, available)))
			continue
		}
		descriptionWidth := max(1, available-nameColumnWidth-2)
		description = truncateWithEllipsis(description, descriptionWidth)
		gap := strings.Repeat(" ", max(2, nameColumnWidth-nameWidth+2))
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
	index := max(0, min(v.picker.GlobalIndex(), len(v.matches)-1))
	total := len(v.matches)
	indicator := ""
	if total > maxSlashRows {
		items := v.picker.VisibleItems()
		start, end := paneWindow(len(items), v.picker.Index(), maxSlashRows, layoutNormal)
		switch {
		case start > 0 && end < total:
			indicator = " ↕"
		case start > 0:
			indicator = " ↑"
		case end < total:
			indicator = " ↓"
		}
	}
	return fmt.Sprintf("%d/%d%s", index+1, total, indicator)
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

func (m *bubbleModel) isCommandLine(line string) bool {
	if !slashview.IsCommandLine(line) {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if _, err := os.Stat(trimmed); err == nil {
		parsed := slashview.ParseCommand(trimmed)
		if _, ok := slashview.LookupCommand(parsed.Name); ok {
			return true
		}
		if m != nil && m.isSkillCommand(parsed.Name) {
			return true
		}
		return false
	}
	return true
}

func isCommandLine(line string) bool {
	if !slashview.IsCommandLine(line) {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if _, err := os.Stat(trimmed); err == nil {
		parsed := slashview.ParseCommand(trimmed)
		if _, ok := slashview.LookupCommand(parsed.Name); !ok {
			return false
		}
	}
	return true
}

func parseCommand(line string) slashview.ParsedCommand {
	return slashview.ParseCommand(line)
}

type slashContext = slashview.Context

const (
	slashKindCommand = slashview.ContextCommand
	slashKindSkill   = slashview.ContextSkill
	slashKindLow     = slashview.ContextLowConcurrency
)

func (m *bubbleModel) parseSlashContext() (slashContext, bool) {
	if m.panes.bottom == nil || m.panes.bottom.bashMode() || m.panes.bottom.has(permissionViewID) || m.panes.bottom.has(skillsViewID) {
		return slashContext{}, false
	}
	prompt := m.panes.bottom.prompt()
	if prompt == nil {
		return slashContext{}, false
	}
	return slashview.ParseContext(prompt.Value())
}

func (m *bubbleModel) slashMatches() []slashCommand {
	context, ok := m.parseSlashContext()
	if !ok {
		return nil
	}
	var items []slashview.Skill
	if m.skills != nil {
		skills := m.skills.List()
		items = make([]slashview.Skill, 0, len(skills))
		for _, skill := range skills {
			items = append(items, slashview.Skill{
				Name:        skill.Name,
				Description: skill.Description,
				Scope:       string(skill.Scope),
				Active:      m.skills.IsActivated(skill.Name),
			})
		}
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
	m.panes.bottom.syncPromptChrome()
	matches := m.slashMatches()
	if len(matches) == 0 {
		m.panes.bottom.remove(slashViewID)
		m.syncMentionView()
		return
	}
	m.panes.bottom.remove(mentionViewID)
	view := m.slashState()
	if view == nil {
		view = &slashPaneView{}
		m.panes.bottom.push(view)
	}
	view.sync(newPaneRenderContext(m))
}

func (m *bubbleModel) slashOpen() bool {
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

const maxMentionRows = 6

const mentionViewID = "mention"

type mentionListItem struct {
	item mentionview.Item
}

func (i mentionListItem) FilterValue() string { return i.item.FilterValue() }
func (i mentionListItem) Title() string       { return i.item.Title() }
func (i mentionListItem) Description() string { return i.item.DisplayDescription() }

type mentionItemDelegate struct{}

func (mentionItemDelegate) Height() int                         { return 1 }
func (mentionItemDelegate) Spacing() int                        { return 0 }
func (mentionItemDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (mentionItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(mentionListItem)
	if !ok {
		return
	}
	prefix := "  "
	nameStyle := bodyStyle
	if index == m.Index() {
		prefix = tuistyle.SelectionStyle.Render(glyphPrompt)
		nameStyle = tuistyle.SelectionStyle
	}
	name := entry.Title()
	description := entry.Description()
	available := max(1, m.Width()-2)
	nameWidth := len([]rune(name))
	if description == "" || available-nameWidth < 8 {
		_, _ = fmt.Fprint(w, prefix+nameStyle.Render(truncateWithEllipsis(name, available)))
		return
	}
	description = truncateWithEllipsis(description, max(1, available-nameWidth-2))
	_, _ = fmt.Fprint(w, prefix+nameStyle.Render(name)+"  "+mutedStyle.Render(description))
}

func mentionPickerWidth(width int) int {
	return max(1, width-4)
}

type mentionPaneView struct {
	picker  list.Model
	ready   bool
	matches []mentionview.Item
}

func (*mentionPaneView) ID() string                             { return mentionViewID }
func (*mentionPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *mentionPaneView) sync(ctx paneRenderContext) {
	matches := ctx.mentionMatches
	v.matches = append(v.matches[:0], matches...)
	items := make([]list.Item, 0, len(matches))
	for _, match := range matches {
		items = append(items, mentionListItem{item: match})
	}
	if !v.ready {
		v.picker = paneutil.NewMinimalList(items, mentionItemDelegate{}, mentionPickerWidth(ctx.width), maxMentionRows)
		v.picker.SetFilteringEnabled(false)
		v.picker.InfiniteScrolling = true
		v.ready = true
	} else {
		_ = v.picker.SetItems(items)
	}
	visibleRows := min(maxMentionRows, len(matches))
	v.picker.SetSize(mentionPickerWidth(ctx.width), max(1, visibleRows))
	if len(matches) == 0 {
		return
	}
	selected := max(0, min(v.picker.Index(), len(matches)-1))
	v.picker.Select(selected)
}

func (v *mentionPaneView) Render(ctx paneRenderContext) string {
	if !v.ready || len(v.matches) == 0 {
		return ""
	}
	rows := v.itemRows(ctx)
	if layoutModeForHeight(ctx.height) != layoutTiny {
		width := panecommon.PaneHelpWidth(ctx.width)
		statusText := v.selectionStatusText()
		helpWidth := max(1, width-ansi.StringWidth(statusText)-1)
		rows = append(rows, paneHelpStatusLine(width, mentionPickerHelp(helpWidth), statusText))
	} else if status := v.selectionStatus(ctx.width); status != "" {
		rows = append(rows, status)
	}
	return strings.Join(rows, "\n")
}

func (v *mentionPaneView) itemRows(ctx paneRenderContext) []string {
	items := v.picker.VisibleItems()
	if len(items) == 0 {
		return nil
	}
	start, end := paneWindow(len(items), v.picker.Index(), maxMentionRows, layoutModeForHeight(ctx.height))
	rows := make([]string, 0, end-start)
	available := panecommon.PaneHelpWidth(ctx.width)
	nameColumnWidth := 0
	for i := start; i < end; i++ {
		entry, ok := items[i].(mentionListItem)
		if !ok {
			continue
		}
		nameColumnWidth = max(nameColumnWidth, len([]rune(entry.Title())))
	}
	for i := start; i < end; i++ {
		entry, ok := items[i].(mentionListItem)
		if !ok {
			continue
		}
		prefix := "  "
		nameStyle := bodyStyle
		if i == v.picker.Index() {
			prefix = tuistyle.SelectionStyle.Render(glyphPrompt)
			nameStyle = tuistyle.SelectionStyle
		}
		name := entry.Title()
		description := entry.Description()
		nameWidth := len([]rune(name))
		if description == "" || available-nameColumnWidth < 8 {
			rows = append(rows, prefix+nameStyle.Render(truncateWithEllipsis(name, available)))
			continue
		}
		descriptionWidth := max(1, available-nameColumnWidth-2)
		description = truncateWithEllipsis(description, descriptionWidth)
		gap := strings.Repeat(" ", max(2, nameColumnWidth-nameWidth+2))
		rows = append(rows, prefix+nameStyle.Render(name)+gap+mutedStyle.Render(description))
	}
	return rows
}

func mentionPickerHelp(width int) string {
	return paneKeyboardHelp(width, "↑/↓", "Navigate", "enter/tab", "Select", "esc", "Dismiss")
}

func (v *mentionPaneView) selectionStatusText() string {
	if len(v.matches) == 0 {
		return ""
	}
	index := max(0, min(v.picker.GlobalIndex(), len(v.matches)-1))
	total := len(v.matches)
	indicator := ""
	if total > maxMentionRows {
		items := v.picker.VisibleItems()
		start, end := paneWindow(len(items), v.picker.Index(), maxMentionRows, layoutNormal)
		switch {
		case start > 0 && end < total:
			indicator = " ↕"
		case start > 0:
			indicator = " ↑"
		case end < total:
			indicator = " ↓"
		}
	}
	return fmt.Sprintf("%d/%d%s", index+1, total, indicator)
}

func (v *mentionPaneView) selectionStatus(width int) string {
	return paneRightStatus(width, v.selectionStatusText())
}

func (v *mentionPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.sync(ctx)
	switch {
	case key.Matches(message, paneutil.Keys.CompletionNav):
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	case key.Matches(message, paneutil.Keys.Tab), key.Matches(message, paneutil.Keys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionAcceptMention}}
	case key.Matches(message, paneutil.Keys.CompletionClose):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: mentionViewID}}
	default:
		return paneKeyResult{}
	}
}

// Workspace file index cache
type workspaceFileCache struct {
	workDir     string
	items       []mentionview.Item
	lastIndexed time.Time
}

const (
	maxWorkspaceScanItems = 3000
	maxWorkspaceScanDepth = 8
	workspaceCacheTTL     = 15 * time.Second
)

func scanWorkspaceFiles(workDir string) []mentionview.Item {
	if strings.TrimSpace(workDir) == "" {
		return nil
	}

	ignorePatterns := loadGitIgnore(workDir)
	var items []mentionview.Item

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxWorkspaceScanDepth || len(items) >= maxWorkspaceScanItems {
			return
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}

		for _, entry := range entries {
			name := entry.Name()
			if isIgnoredDefault(name) {
				continue
			}

			fullPath := filepath.Join(dir, name)
			relPath, err := filepath.Rel(workDir, fullPath)
			if err != nil {
				continue
			}

			if matchesAnyPattern(relPath, name, ignorePatterns) {
				continue
			}

			if entry.IsDir() {
				items = append(items, mentionview.Item{
					Kind:        mentionview.ItemKindDir,
					Name:        relPath,
					Description: "Directory",
					PrefixTag:   "[dir]",
				})
				walk(fullPath, depth+1)
			} else {
				info, err := entry.Info()
				sizeStr := ""
				if err == nil {
					sz := info.Size()
					if sz >= 1024*1024 {
						sizeStr = fmt.Sprintf("%.1f MB", float64(sz)/(1024*1024))
					} else if sz >= 1024 {
						sizeStr = fmt.Sprintf("%d KB", sz/1024)
					} else {
						sizeStr = fmt.Sprintf("%d B", sz)
					}
				}
				items = append(items, mentionview.Item{
					Kind:        mentionview.ItemKindFile,
					Name:        relPath,
					Description: sizeStr,
					PrefixTag:   "[file]",
				})
			}

			if len(items) >= maxWorkspaceScanItems {
				return
			}
		}
	}

	walk(workDir, 0)
	return items
}

func isIgnoredDefault(name string) bool {
	switch name {
	case ".git", "node_modules", "target", "bin", "dist", ".idea", ".vscode", ".bak":
		return true
	}
	return strings.HasPrefix(name, ".") && len(name) > 1
}

func loadGitIgnore(workDir string) []string {
	path := filepath.Join(workDir, ".gitignore")
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func matchesAnyPattern(relPath, baseName string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimPrefix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if glob.Match(p, relPath) || glob.Match(p, baseName) {
			return true
		}
	}
	return false
}

func (m *bubbleModel) getWorkspaceMentionItems() []mentionview.Item {
	if m == nil || m.workDir == "" {
		return nil
	}
	if m.mentionCache.workDir == m.workDir && time.Since(m.mentionCache.lastIndexed) < workspaceCacheTTL && len(m.mentionCache.items) > 0 {
		return m.mentionCache.items
	}

	items := scanWorkspaceFiles(m.workDir)
	m.mentionCache = workspaceFileCache{
		workDir:     m.workDir,
		items:       items,
		lastIndexed: time.Now(),
	}
	return items
}

func (m *bubbleModel) cursorOffset() int {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return 0
	}
	prompt := m.panes.bottom.prompt()
	val := prompt.Value()
	lines := strings.Split(val, "\n")
	currLine := prompt.Line()
	currCol := prompt.Column()
	offset := 0
	for i := 0; i < currLine && i < len(lines); i++ {
		offset += len([]rune(lines[i])) + 1 // +1 for '\n'
	}
	if currLine < len(lines) {
		lineRunes := []rune(lines[currLine])
		if currCol > len(lineRunes) {
			currCol = len(lineRunes)
		}
		offset += currCol
	}
	return offset
}

func (m *bubbleModel) textBeforeCursor() string {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return ""
	}
	runes := []rune(m.panes.bottom.prompt().Value())
	offset := m.cursorOffset()
	if offset > len(runes) {
		offset = len(runes)
	}
	return string(runes[:offset])
}

func (m *bubbleModel) parseMentionContext() (mentionview.Context, bool) {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.bashMode() || m.panes.bottom.has(permissionViewID) || m.panes.bottom.has(skillsViewID) {
		return mentionview.Context{}, false
	}
	if m.slashOpen() {
		return mentionview.Context{}, false
	}
	text := m.textBeforeCursor()
	return mentionview.ParseContext(text)
}

func (m *bubbleModel) mentionMatches() []mentionview.Item {
	ctx, ok := m.parseMentionContext()
	if !ok {
		return nil
	}
	agents := mentionview.DefaultAgents()
	workspaceFiles := m.getWorkspaceMentionItems()
	return mentionview.Matches(ctx, agents, workspaceFiles)
}

func (m *bubbleModel) mentionState() *mentionPaneView {
	if m.panes.bottom == nil {
		return nil
	}
	view, _ := m.panes.bottom.find(mentionViewID).(*mentionPaneView)
	return view
}

func (m *bubbleModel) syncMentionView() {
	if m.panes.bottom == nil {
		return
	}
	matches := m.mentionMatches()
	if len(matches) == 0 {
		m.panes.bottom.remove(mentionViewID)
		return
	}
	view := m.mentionState()
	if view == nil {
		view = &mentionPaneView{}
		m.panes.bottom.push(view)
	}
	view.sync(newPaneRenderContext(m))
}

func (m *bubbleModel) mentionOpen() bool {
	return m.panes.bottom != nil && m.panes.bottom.has(mentionViewID)
}

func (m *bubbleModel) clampMentionIndex() {
	m.syncMentionView()
	if view := m.mentionState(); view != nil {
		view.sync(newPaneRenderContext(m))
	}
}

func (m *bubbleModel) acceptMention() (applied bool, command tea.Cmd) {
	m.syncMentionView()
	view := m.mentionState()
	matches := m.mentionMatches()
	if view == nil || len(matches) == 0 {
		return false, nil
	}
	m.clampMentionIndex()
	view = m.mentionState()
	if view == nil || len(view.matches) == 0 {
		return false, nil
	}

	selected := view.matches[view.picker.Index()]
	ctx, ok := m.parseMentionContext()
	if !ok {
		m.panes.bottom.remove(mentionViewID)
		return false, nil
	}

	prompt := m.panes.bottom.prompt()
	fullVal := prompt.Value()
	runes := []rune(fullVal)

	if selected.Kind == mentionview.ItemKindFile && mentionview.IsImageFile(selected.Name) {
		fullPath := selected.Name
		if !filepath.IsAbs(fullPath) && m.workDir != "" {
			fullPath = filepath.Join(m.workDir, fullPath)
		}
		newRunes := append(runes[:ctx.StartOffset], runes[ctx.EndOffset:]...)
		prompt.SetValue(string(newRunes))
		prompt.CursorEnd()
		m.panes.bottom.attachImage(fullPath)
		m.panes.bottom.remove(mentionViewID)
		return true, nil
	}

	insertionText := selected.InsertionText()
	insertionRunes := []rune(insertionText)

	var newRunes []rune
	if ctx.StartOffset <= len(runes) && ctx.EndOffset <= len(runes) && ctx.StartOffset <= ctx.EndOffset {
		newRunes = append(runes[:ctx.StartOffset], append(insertionRunes, runes[ctx.EndOffset:]...)...)
	} else {
		newRunes = append(runes, insertionRunes...)
	}

	prompt.SetValue(string(newRunes))

	targetOffset := ctx.StartOffset + len(insertionRunes)
	if targetOffset >= len(newRunes) {
		prompt.CursorEnd()
	} else {
		lines := strings.Split(string(newRunes[:targetOffset]), "\n")
		targetRow := len(lines) - 1
		targetCol := len([]rune(lines[targetRow]))
		prompt.MoveToBegin()
		for r := 0; r < targetRow; r++ {
			prompt.CursorDown()
		}
		prompt.SetCursorColumn(targetCol)
	}

	if selected.Kind == mentionview.ItemKindDir {
		m.syncMentionView()
	} else {
		m.panes.bottom.remove(mentionViewID)
	}

	return true, nil
}
