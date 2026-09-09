package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
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
		prefix += " "
	}
	return prefix + i.command.Name
}
func (i slashListItem) Description() string {
	if i.command.Scope != "" {
		return i.command.Description + " · " + i.command.Scope
	}
	return i.command.Description
}

type slashPaneView struct {
	index   int
	picker  list.Model
	ready   bool
	matches []slashCommand
}

func (*slashPaneView) ID() string             { return slashViewID }
func (*slashPaneView) ReplacesComposer() bool { return false }

func (v *slashPaneView) sync(m *bubbleModel) {
	matches := m.slashMatches()
	v.matches = append(v.matches[:0], matches...)
	items := make([]list.Item, 0, len(matches))
	for _, command := range matches {
		items = append(items, slashListItem{command: command})
	}
	if !v.ready {
		delegate := list.NewDefaultDelegate()
		delegate.SetSpacing(0)
		v.picker = list.New(items, delegate, maxInt(20, m.width-4), maxInt(4, minInt(12, m.height/2)))
		v.picker.DisableQuitKeybindings()
		v.picker.SetFilteringEnabled(false)
		v.picker.SetShowTitle(false)
		v.picker.SetShowStatusBar(false)
		v.picker.SetShowPagination(false)
		v.picker.SetShowHelp(false)
		v.picker.InfiniteScrolling = true
		v.ready = true
	} else {
		_ = v.picker.SetItems(items)
	}
	if len(matches) == 0 {
		v.index = 0
		return
	}
	v.index = maxInt(0, minInt(v.index, len(matches)-1))
	v.picker.Select(v.index)
}

func (v *slashPaneView) Render(m *bubbleModel) string {
	v.sync(m)
	if len(v.matches) == 0 {
		return ""
	}
	v.picker.SetSize(maxInt(20, m.width-4), maxInt(4, minInt(12, m.height/2)))
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = layoutModeForHeight(m.height) == layoutNormal
	v.picker.SetDelegate(delegate)
	return v.picker.View()
}

func (v *slashPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	v.sync(m)
	switch message.String() {
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.index = v.picker.Index()
		return true, cmd
	case "tab":
		_, command := m.acceptSlash(false)
		return true, command
	case "enter":
		_, command := m.acceptSlash(true)
		return true, command
	case "esc":
		m.bottom.remove(slashViewID)
		return true, nil
	default:
		return false, nil
	}
}

func isCommandLine(line string) bool {
	return slashview.IsCommandLine(line)
}

func splitCommand(line string) (string, string, []string) {
	return slashview.SplitCommand(line)
}

func canonicalSlashName(name string) string {
	return slashview.CanonicalName(slashCatalog, name)
}

func fuzzyContains(target, query string) bool {
	return slashview.FuzzyContains(target, query)
}

type slashContext = slashview.Context

const (
	slashKindCommand = slashview.ContextCommand
	slashKindSkill   = slashview.ContextSkill
)

func (m bubbleModel) parseSlashContext() (slashContext, bool) {
	if m.bottom == nil || m.bottom.bashMode() || m.bottom.has(permissionViewID) || m.bottom.has(skillsViewID) {
		return slashContext{}, false
	}
	prompt := m.bottom.prompt()
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
	if m.bottom == nil {
		return nil
	}
	view, _ := m.bottom.find(slashViewID).(*slashPaneView)
	return view
}

func (m *bubbleModel) syncSlashView() {
	if m.bottom == nil {
		return
	}
	matches := m.slashMatches()
	if len(matches) == 0 {
		m.bottom.remove(slashViewID)
		return
	}
	view := m.slashState()
	if view == nil {
		view = &slashPaneView{}
		m.bottom.push(view)
	}
	view.sync(m)
}

func (m bubbleModel) slashOpen() bool {
	return len(m.slashMatches()) > 0
}

func (m *bubbleModel) clampSlashIndex() {
	m.syncSlashView()
	if view := m.slashState(); view != nil {
		view.sync(m)
	}
}

func (m *bubbleModel) moveSlash(delta int) {
	m.syncSlashView()
	view := m.slashState()
	if view == nil || len(view.matches) == 0 {
		return
	}
	if delta < 0 {
		view.picker.CursorUp()
	} else if delta > 0 {
		view.picker.CursorDown()
	}
	view.index = view.picker.Index()
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
	view.index = view.picker.Index()
	selected := view.matches[view.index]
	context, _ := m.parseSlashContext()
	prompt := m.bottom.prompt()
	var insertion string
	if context.Kind == slashKindSkill {
		insertion = context.Lead + selected.Name
	} else {
		insertion = context.Prefix + selected.Name
		if selected.TakesArgs {
			insertion += " "
			prompt.SetValue(insertion)
			prompt.CursorEnd()
			m.bottom.remove(slashViewID)
			return true, nil
		}
	}
	if !run {
		prompt.SetValue(insertion)
		prompt.CursorEnd()
		m.bottom.remove(slashViewID)
		return true, nil
	}
	m.resetPrompt()
	m.bottom.remove(slashViewID)
	return true, m.dispatch(insertion)
}

func truncateWithEllipsis(s string, maxLen int) string {
	return textview.TruncateEllipsis(s, maxLen)
}

func (m bubbleModel) renderSlash(index int) string {
	view := &slashPaneView{index: index}
	return view.Render(&m)
}

func (m bubbleModel) slashView() string {
	if view := m.slashState(); view != nil {
		return view.Render(&m)
	}
	return ""
}

func (m *bubbleModel) executeCommand(line string) tea.Cmd {
	rawName, argument, parts := splitCommand(line)
	name := canonicalSlashName(rawName)
	switch name {
	case "help":
		m.appendHelp()
	case "tools":
		m.appendRegisteredTools()
	case "skills":
		return m.handleSkillsCommand(argument, parts)
	case "project":
		return m.executeProjectCommand(line, rawName)
	case "config":
		return m.executeUserConfigCommand(line, rawName)
	case "session", "sessions":
		return m.executeSessionCommand(name)
	case "mode", "ask", "always-approve", "plan":
		return m.executePermissionCommand(name, argument)
	case "transcript", "todo", "clear", "new":
		return m.executeConversationCommand(name, argument)
	case "model":
		return m.executeModelCommand(argument)
	case "provider":
		return m.executeProviderCommand(line, rawName)
	case "agents":
		return m.openAgentsPane()
	case "subagents":
		return m.handleSubagentsCommand(argument)
	case "agent":
		return m.handleAgentCommand(argument)
	case "reasoning":
		return m.handleReasoningCommand(argument)
	case "call":
		return m.startCall(parts)
	case "quit":
		return tea.Quit
	default:
		m.appendError(fmt.Sprintf("unknown command %q; try /help", name))
	}
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) appendHelp() {
	for _, command := range slashCatalog {
		alias := ""
		if len(command.Aliases) > 0 {
			alias = " (" + strings.Join(prefixNames(command.Aliases), ", ") + ")"
		}
		m.appendLine("/" + textview.PadRight(command.Name, 16) + " " + command.Description + alias)
	}
}

func prefixNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, "/"+name)
	}
	return out
}

func (m *bubbleModel) handleAgentCommand(argument string) tea.Cmd {
	arg := strings.TrimSpace(argument)
	if arg == "" {
		current := m.agentProfile
		if current == "" {
			current = "universal"
		}
		m.appendLine(fmt.Sprintf("Active agent profile: %s", commandStyle.Render(current)))
		m.appendLine("Available profiles:")
		m.appendLine("  universal - Primary adaptive software engineering orchestrator")
		m.appendLine("  strength - Substantial implementation, fixes, and refactors")
		m.appendLine("  agility - Fast read-only exploration and tracing")
		m.appendLine("  intelligence - Deep reasoning, architecture, and high-risk engineering")
		m.appendLine("Switch profile: /agent <" + agent.ProfileList("|") + ">")
		m.refreshViewport()
		return nil
	}
	prof, err := agent.ParseProfile(arg)
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	m.agentProfile = string(prof)
	m.reconfigureRunner()
	m.appendLine(successStyle.Render(fmt.Sprintf("Agent profile switched to %s.", prof)))
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) executeConversationCommand(name, argument string) tea.Cmd {
	switch name {
	case "transcript":
		m.showTranscript = true
		m.refreshTranscriptViewport(true)
	case "todo":
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			m.toggleTodoPane()
		case "show":
			if !m.bottom.has(todoInspectViewID) {
				m.bottom.push(&todoPaneView{})
			}
			m.relayout()
		case "hide":
			m.bottom.remove(todoInspectViewID)
			m.relayout()
		default:
			m.appendError("usage: /todo [show|hide]")
		}
	case "clear":
		m.resetTranscript()
		m.refreshViewport()
	case "new":
		m.resetConversation()
		m.refreshViewport()
	}
	return nil
}

func (m *bubbleModel) selectModelDirect(modelID string) tea.Cmd {
	prov := m.activeProvider
	if prov == "" {
		if len(m.providers) > 0 {
			for name := range m.providers {
				prov = name
				break
			}
		} else {
			prov = model.DefaultProtonmanName
		}
	}
	return saveModelSelectionCmd(prov, modelID, !m.modelIDKnown(prov, modelID))
}

func (m *bubbleModel) executeModelCommand(argument string) tea.Cmd {
	arg := strings.TrimSpace(argument)
	switch arg {
	case "add":
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneView())
			m.relayout()
		}
		return nil
	case "free":
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneViewWithPreset(model.DefaultOpenCodeName))
			m.relayout()
		}
		return nil
	case "", "select":
		return m.openModelSelectPane()
	default:
		return m.selectModelDirect(arg)
	}
}

func (m *bubbleModel) executeProviderCommand(line, rawName string) tea.Cmd {
	cmdLine := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	cmdLine = strings.TrimSpace(strings.TrimPrefix(cmdLine, rawName))
	fields := strings.Fields(cmdLine)
	subCmd, preset := "", ""
	if len(fields) > 0 {
		subCmd = fields[0]
	}
	if len(fields) > 1 {
		preset = fields[1]
	}
	if subCmd == "add" {
		if !m.bottom.has(providerViewID) {
			if preset != "" {
				m.bottom.push(newProviderPaneViewWithPreset(preset))
			} else {
				m.bottom.push(newProviderPaneView())
			}
			m.relayout()
		}
		return nil
	}
	if subCmd == "" || subCmd == "select" {
		if !m.bottom.has(providerSelectViewID) {
			m.bottom.push(newProviderSelectPaneView(m))
			m.relayout()
		}
		return nil
	}
	if subCmd == "list" {
		m.appendProviderList()
		return nil
	}
	for name := range m.providers {
		if strings.EqualFold(name, subCmd) {
			return saveActiveProviderCmd(name)
		}
	}
	if p := model.LookupPreset(subCmd); p != nil {
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneViewWithPreset(p.ID))
			m.relayout()
		}
		return nil
	}
	m.appendLine(mutedStyle.Render(fmt.Sprintf("unknown provider %q; try /provider, /provider list, or /provider add", subCmd)))
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) appendProviderList() {
	if len(m.providers) == 0 {
		m.appendLine(mutedStyle.Render("No providers configured yet. Use /provider to see supported providers."))
	} else {
		m.appendLine(brandStyle.Render("Configured Providers:"))
		for name, p := range m.providers {
			activeTag := ""
			if strings.EqualFold(name, m.activeProvider) {
				activeTag = " " + successStyle.Render("[active]")
			}
			m.appendLine(fmt.Sprintf("  • %s: %s%s", name, p.BaseURL, activeTag))
		}
		if m.activeModel != "" {
			m.appendLine(mutedStyle.Render(fmt.Sprintf("Active model: %s", m.activeModel)))
		}
	}
	m.refreshViewport()
}

func (m *bubbleModel) executePermissionCommand(name, argument string) tea.Cmd {
	switch name {
	case "mode":
		if argument == "" {
			m.appendLine("permission mode: " + m.service.Mode().String())
			m.refreshViewport()
			return nil
		}
		mode, err := permission.ParseMode(argument)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		if err := m.setPermissionMode(mode); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		if mode == permission.ModeAlwaysApprove {
			m.setPlanEnabled(false)
		}
		m.appendLine("permission mode: " + mode.String())
	case "ask":
		if err := m.setPermissionMode(permission.ModeAsk); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.setPlanEnabled(false)
		m.appendLine("permission mode: " + permission.ModeAsk.String())
	case "always-approve":
		if err := m.setPermissionMode(permission.ModeAlwaysApprove); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.setPlanEnabled(false)
		m.appendLine("permission mode: " + permission.ModeAlwaysApprove.String())
	case "plan":
		m.setPlanMode(argument)
	}
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) executeProjectCommand(line, rawName string) tea.Cmd {
	cmdLine := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	cmdLine = strings.TrimSpace(strings.TrimPrefix(cmdLine, rawName))
	fields := strings.Fields(cmdLine)
	subCmd := ""
	if len(fields) > 0 {
		subCmd = strings.ToLower(fields[0])
	}
	switch subCmd {
	case "", "status", "reload":
		return m.openProjectPane()
	case "init":
		return m.initProject()
	case "set":
		settingArgs := ""
		if len(fields) > 1 {
			settingArgs = strings.Join(fields[1:], " ")
		}
		return m.handleProjectSet(settingArgs)
	case "permission":
		permArgs := ""
		if len(fields) > 1 {
			permArgs = strings.Join(fields[1:], " ")
		}
		return m.handleProjectPermission(permArgs)
	default:
		m.appendError("usage: /project [status|reload|init|set <setting> <value>|permission <allow|deny|ask> <tool> [pattern]]")
		m.refreshViewport()
		return nil
	}
}

func (m *bubbleModel) executeSessionCommand(name string) tea.Cmd {
	if name == "session" {
		m.appendLine("session: " + m.sessionID)
		if m.workspaceKey != "" {
			m.appendLine("workspace: " + m.workspaceKey)
		}
		m.appendLine(fmt.Sprintf("messages: %d", len(m.messages)))
		m.refreshViewport()
		return nil
	}
	if m.sessions == nil {
		m.appendError("session store is unavailable")
		m.refreshViewport()
		return nil
	}
	summaries, err := m.sessions.ListSummaries(m.ctx, app.SessionListOptions{WorkspaceKey: m.workspaceKey, Limit: 20})
	if err != nil {
		m.appendError("list sessions: " + err.Error())
		m.refreshViewport()
		return nil
	}
	if len(summaries) == 0 {
		m.appendLine("No resumable sessions for this workspace.")
		m.refreshViewport()
		return nil
	}
	m.appendLine("Recent sessions:")
	for _, summary := range summaries {
		marker := " "
		if summary.ID == m.sessionID {
			marker = "*"
		}
		profile := summary.AgentProfile
		if profile == "" {
			profile = "-"
		}
		preview := summary.Preview
		if preview == "" {
			preview = "(empty session)"
		}
		m.appendLine(fmt.Sprintf("%s %s  %s  %s", marker, summary.ID, profile, truncateWithEllipsis(preview, 72)))
	}
	m.appendLine("Resume with: protonman session resume <session-id>")
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) handleSkillsCommand(argument string, parts []string) tea.Cmd {
	trimmedArg := strings.TrimSpace(argument)
	if m.skills == nil || len(m.skills.List()) == 0 {
		m.appendLine("No agent skills discovered.")
		m.appendLine(fmt.Sprintf("Place skills in %s or .protonman/skills/ (with PROTONMAN_TRUST_PROJECT=1).", appdirs.UserSkillsDisplay()))
		m.refreshViewport()
		return nil
	}
	if trimmedArg == "" {
		skillsList := m.skills.List()
		activeCount := len(m.skills.ActivatedList())
		m.appendLine(fmt.Sprintf("Agent Skills (%d/%d active):", activeCount, len(skillsList)))
		maxPrint := 8
		if len(skillsList) <= maxPrint {
			for _, s := range skillsList {
				box := "[ ]"
				if m.skills.IsActivated(s.Name) {
					box = "[x]"
				}
				m.appendLine(fmt.Sprintf("  %s %s", box, s.Name))
			}
		} else {
			printed := 0
			for _, s := range skillsList {
				if m.skills.IsActivated(s.Name) {
					m.appendLine(fmt.Sprintf("  [x] %s", s.Name))
					printed++
				}
			}
			for _, s := range skillsList {
				if printed >= maxPrint {
					break
				}
				if !m.skills.IsActivated(s.Name) {
					m.appendLine(fmt.Sprintf("  [ ] %s", s.Name))
					printed++
				}
			}
			remaining := len(skillsList) - printed
			if remaining > 0 {
				m.appendLine(fmt.Sprintf("  … and %d more skills. (Browse all in picker below, or use /skills <name>)", remaining))
			}
		}
		m.bottom.push(&skillsPaneView{})
		m.relayout()
		return nil
	}
	if trimmedArg == "active" {
		active := m.skills.ActivatedList()
		if len(active) == 0 {
			m.appendLine("No active agent skills in this session.")
			m.appendLine("Activate skills using /skills <name> or the skill tool.")
		} else {
			m.appendLine(fmt.Sprintf("Active Agent Skills (%d):", len(active)))
			for _, name := range active {
				m.appendLine(fmt.Sprintf("  [x] %s", name))
			}
		}
		m.refreshViewport()
		return nil
	}
	if trimmedArg == "toggle" {
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			m.appendError("usage: /skill toggle <name>")
			m.refreshViewport()
			return nil
		}
		target := strings.TrimSpace(parts[2])
		active, err := m.skills.Toggle(target)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		state := "deactivated"
		box := "[ ]"
		if active {
			state = "activated"
			box = "[x]"
		}
		m.appendLine(fmt.Sprintf("%s Skill %q %s.", box, target, state))
		m.refreshViewport()
		return nil
	}
	if trimmedArg == "deactivate" || trimmedArg == "disable" || trimmedArg == "remove" || trimmedArg == "off" {
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			m.appendError(fmt.Sprintf("usage: /skill %s <name>", trimmedArg))
			m.refreshViewport()
			return nil
		}
		target := strings.TrimSpace(parts[2])
		if _, ok := m.skills.Lookup(target); !ok {
			m.appendError(fmt.Sprintf("skill %q not found; try /skills to list available skills", target))
			m.refreshViewport()
			return nil
		}
		if !m.skills.IsActivated(target) {
			m.appendLine(fmt.Sprintf("[ ] Skill %q is not active.", target))
			m.refreshViewport()
			return nil
		}
		m.skills.Deactivate(target)
		m.appendLine(fmt.Sprintf("[ ] Skill %q deactivated.", target))
		m.refreshViewport()
		return nil
	}
	target := trimmedArg
	if (trimmedArg == "activate" || trimmedArg == "enable" || trimmedArg == "on") && len(parts) >= 3 {
		target = strings.TrimSpace(parts[2])
	}
	s, ok := m.skills.Lookup(target)
	if !ok {
		m.appendError(fmt.Sprintf("skill %q not found; try /skills to list available skills", target))
		m.refreshViewport()
		return nil
	}
	if m.skills.IsActivated(s.Name) {
		m.appendLine(fmt.Sprintf("[x] Skill %q is already active. Use /skill toggle %s to deactivate.", s.Name, s.Name))
		m.refreshViewport()
		return nil
	}
	m.skills.MarkActivated(s.Name)
	m.appendLine(fmt.Sprintf("[x] Activated skill %s [%s]: %s", s.Name, s.Scope, s.Description))
	if len(s.Resources) > 0 {
		m.appendLine("Bundled resources:")
		for _, r := range s.Resources {
			m.appendLine("  - " + r)
		}
	}
	m.refreshViewport()
	return nil
}

func parseSubagentsEnabled(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "enable", "enabled":
		return true, nil
	case "off", "false", "disable", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("subagents must be on or off")
	}
}

func subagentsEnabledLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func (m *bubbleModel) handleSubagentsCommand(argument string) tea.Cmd {
	arg := strings.TrimSpace(argument)
	if arg == "" {
		m.appendLine("Subagents: " + commandStyle.Render(subagentsEnabledLabel(m.subagentsEnabled)))
		if !m.subagentsEnabled && len(m.agents.List()) > 0 {
			m.appendMuted("New delegation is disabled; existing agents remain manageable.")
		}
		m.refreshViewport()
		return nil
	}
	enabled, err := parseSubagentsEnabled(arg)
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	m.subagentsEnabled = enabled
	m.agents.SetEnabled(enabled)
	m.reconfigureRunner()
	if enabled {
		m.appendLine(successStyle.Render("Subagents enabled."))
	} else {
		m.appendLine(successStyle.Render("Subagents disabled."))
		if len(m.agents.List()) > 0 {
			m.appendMuted("Running and retained agents remain available for lifecycle control.")
		} else {
			m.appendMuted("Universal will handle work directly.")
		}
	}
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) appendRegisteredTools() {
	m.appendLine("Registered tools:")
	for _, definition := range m.registry.Definitions() {
		m.appendLine(fmt.Sprintf("- %s [%s]: %s", definition.Name, definition.Kind, definition.Description))
	}
}

func (m *bubbleModel) startCall(parts []string) tea.Cmd {
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		m.appendError("usage: /call <tool> <json>")
		m.refreshViewport()
		return nil
	}
	arguments := "{}"
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		arguments = parts[2]
	}
	m.nextID++
	call, err := tool.NewCall(fmt.Sprintf("bubble-%d", m.nextID), strings.TrimSpace(parts[1]), []byte(arguments))
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	return m.startTool(call)
}

func (m *bubbleModel) startBash(command string) tea.Cmd {
	m.nextID++
	payload := fmt.Sprintf(`{"command":%q}`, command)
	call, err := tool.NewCall(fmt.Sprintf("bubble-%d", m.nextID), "bash", []byte(payload))
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	return m.startTool(call)
}
