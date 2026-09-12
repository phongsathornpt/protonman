package runtime

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/cmdpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/transientnotice"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func (m *bubbleModel) executeCommand(line string) tea.Cmd {
	defer m.reconcileLayout()

	cmd := cmdpolicy.Classify(line)
	switch cmd.Kind {
	case cmdpolicy.KindHelp:
		m.appendHelp()
	case cmdpolicy.KindPermission:
		m.openPermissionModePane()
	case cmdpolicy.KindLow:
		return m.handleLowConcurrencyCommand(cmd.Argument)
	case cmdpolicy.KindSkills:
		return m.handleSkillsCommand(cmd.Argument, cmd.Parts)
	case cmdpolicy.KindGoal:
		return m.executeConversationCommand(cmd.Name, cmd.Rest)
	case cmdpolicy.KindClear, cmdpolicy.KindTodo:
		return m.executeConversationCommand(cmd.Name, cmd.Argument)
	case cmdpolicy.KindModel:
		return m.executeModelCommand(cmd.Argument)
	case cmdpolicy.KindProvider:
		return m.executeProviderCommand(line, cmd.Name)
	case cmdpolicy.KindAgents:
		return m.openAgentsPane()
	case cmdpolicy.KindCall:
		return m.startCall(cmd.Parts)
	case cmdpolicy.KindQuit:
		return tea.Quit
	default:
		m.appendError(fmt.Sprintf("unknown command %q; try /help", cmd.Name))
	}
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) appendHelp() {
	for _, command := range slashCatalog {
		m.appendLine("/" + textview.PadRight(command.Name, 16) + " " + command.Description)
	}
}

// Conversation commands (/goal, /clear, /todo)

func (m *bubbleModel) executeConversationCommand(name, argument string) tea.Cmd {
	switch name {
	case "goal":
		return m.handleGoalCommand(argument)
	case "clear":
		if strings.TrimSpace(argument) != "" {
			m.appendError("usage: /clear")
			break
		}
		m.clearConversation()
	case "todo":
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			return m.toggleTodoPane()
		case "show":
			return m.openTodoPane()
		case "hide":
			m.panes.bottom.remove(todoInspectViewID)
			m.requestRelayout()
		default:
			m.appendError("usage: /todo [show|hide]")
		}
	}
	return nil
}

func (m *bubbleModel) handleGoalCommand(argument string) tea.Cmd {
	goal := strings.TrimSpace(argument)
	switch strings.ToLower(goal) {
	case "":
		if m.activeGoal == "" {
			m.appendMuted("no active goal")
			return nil
		}
		m.appendMuted("goal · " + m.activeGoal)
		return nil
	case "clear":
		if err := m.setActiveGoal(""); err != nil {
			m.appendError("failed to clear goal: " + err.Error())
			return nil
		}
		m.appendMuted("goal cleared")
		return nil
	default:
		if m.busy {
			m.appendError("cannot change goal while a turn is running")
			return nil
		}
		if err := m.setActiveGoal(goal); err != nil {
			m.appendError("failed to set goal: " + err.Error())
			return nil
		}
		m.appendMuted("goal · " + goal)
		return m.startTurn(goal)
	}
}

func (m *bubbleModel) setActiveGoal(goal string) error {
	goal = strings.TrimSpace(goal)
	nextRunner := m.runner
	if m.runner != nil {
		runner, err := app.CloneConversationWithGoal(m.runner, goal)
		if err != nil {
			return err
		}
		nextRunner = runner
	}
	if binder, ok := m.todoStore.(tododomain.GoalBoundRepository); ok {
		snapshot, _, err := binder.BindGoal(m.ctx, goal)
		if err != nil {
			return fmt.Errorf("bind task plan to goal: %w", err)
		}
		if m.applyTodoSnapshot(snapshot) {
			m.requestRelayout()
		}
	}
	m.runner = nextRunner
	m.activeGoal = goal
	return nil
}

func (m *bubbleModel) clearConversation() {
	if m.conversation != nil {
		m.conversation.Reset()
	}
	m.conversationViewport = conversationViewportState{mode: viewportFollowing}
	m.ensureHistoryState().Reset()
	m.panes.showTranscript = false
	m.refreshTranscriptViewport(true)
	m.refreshViewport()
	m.appendMuted("conversation cleared")
}

// Low concurrency command (/low)

func (m *bubbleModel) handleLowConcurrencyCommand(argument string) tea.Cmd {
	raw := strings.TrimSpace(argument)
	if raw == "" {
		m.openLowConcurrencyPane()
		return nil
	}
	if m.busy {
		m.appendError("cannot change low concurrency mode while a turn is running")
		return nil
	}

	next, err := model.ParseLowConcurrencySetting(raw)
	if err != nil {
		m.appendError(err.Error())
		return nil
	}
	return m.applyLowConcurrencySetting(next)
}

func (m *bubbleModel) applyLowConcurrencySetting(next model.LowConcurrencySetting) tea.Cmd {
	if m.busy {
		m.appendError("cannot change low concurrency mode while a turn is running")
		return nil
	}
	m.lowConcurrencyMode = next
	m.reconfigureRunner()
	effective := "off"
	if m.lowConcurrencyEffective() {
		effective = "on"
	}
	m.refreshViewport()
	return m.showTransientNotice(fmt.Sprintf("low concurrency · %s · effective %s", next, effective))
}

func (m *bubbleModel) lowConcurrencyEffective() bool {
	if strings.TrimSpace(m.activeModel) == "" {
		return false
	}
	if m.lowConcurrencyMode == model.LowConcurrencyOn {
		return true
	}
	if m.lowConcurrencyMode == model.LowConcurrencyOff {
		return false
	}
	providerName := strings.TrimSpace(m.activeProvider)
	if providerName == "" {
		providerName = model.DefaultOpenCodeName
	}
	provider, ok := m.providers[strings.ToLower(providerName)]
	baseURL := ""
	if ok {
		baseURL = provider.BaseURL
	}
	return model.IsProvider(model.DefaultOpenCodeName, providerName, baseURL) && model.IsFreeModel(m.activeModel)
}

func (m *bubbleModel) lowConcurrencyFooterLabel() string {
	if m.lowConcurrencyEffective() {
		return "LOW"
	}
	return ""
}

func (m *bubbleModel) showTransientNotice(text string) tea.Cmd {
	if m == nil {
		return nil
	}
	m.transientNoticeID++
	id := m.transientNoticeID
	m.transientNotice = strings.TrimSpace(text)
	m.refreshFrameLayout()
	return transientnotice.ExpireAfter(id, transientnotice.DefaultDuration)
}

// Model command (/model)

func (m *bubbleModel) selectModelDirect(modelID string) tea.Cmd {
	providerName := strings.TrimSpace(m.activeProvider)
	if providerName == "" {
		providerName = model.DefaultOpenCodeName
	}
	return m.beginModelSetup(providerName, modelID, !m.modelIDKnown(providerName, modelID))
}

func (m *bubbleModel) executeModelCommand(argument string) tea.Cmd {
	switch arg := strings.TrimSpace(argument); arg {
	case "add":
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneView())
			m.requestRelayout()
		}
		return nil
	case "free":
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneViewWithPreset(model.DefaultOpenCodeName))
			m.requestRelayout()
		}
		return nil
	case "", "select":
		return m.openModelSetupPane()
	default:
		return m.selectModelDirect(arg)
	}
}

// Provider command (/provider)

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
		if !m.panes.bottom.has(providerViewID) {
			if preset != "" {
				m.pushProviderPane(newProviderPaneViewWithPreset(preset))
			} else {
				m.pushProviderPane(newProviderPaneView())
			}
			m.requestRelayout()
		}
		return nil
	}
	if subCmd == "" || subCmd == "select" {
		if !m.panes.bottom.has(providerSelectViewID) {
			m.panes.bottom.push(newProviderSelectPaneView(m))
			m.requestRelayout()
		}
		return nil
	}
	if subCmd == "list" {
		m.appendProviderList()
		return nil
	}
	for name := range m.providers {
		if strings.EqualFold(name, subCmd) {
			return m.beginProviderSelect(name)
		}
	}
	if p := model.LookupPreset(subCmd); p != nil {
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneViewWithPreset(p.ID))
			m.requestRelayout()
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

// Skills commands (/skills)

func (m *bubbleModel) openSkillsPane() tea.Cmd {
	if m.skills == nil || len(m.skills.List()) == 0 {
		m.appendLine("No agent skills discovered.")
		m.appendLine(fmt.Sprintf("Place skills in %s or .protonman/skills/ (with PROTONMAN_TRUST_PROJECT=1).", appdirs.UserSkillsDisplay()))
		m.refreshViewport()
		return nil
	}
	if !m.panes.bottom.has(skillsViewID) {
		m.panes.bottom.push(&skillsPaneView{})
	}
	m.requestRelayout()
	return nil
}

func (m *bubbleModel) toggleSkillsPane() tea.Cmd {
	if m.panes.bottom.has(skillsViewID) {
		m.panes.bottom.remove(skillsViewID)
		m.requestRelayout()
		return nil
	}
	return m.openSkillsPane()
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
		return m.openSkillsPane()
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
	if trimmedArg == "check" || trimmedArg == "verify" {
		lockPath := m.skills.ProjectLockPath()
		if lockPath == "" {
			lockPath = filepath.Join(m.workDir, skill.LockFileName)
		}
		lock, err := skill.ReadLockFile(lockPath)
		if err != nil || len(lock.Skills) == 0 {
			m.appendLine(fmt.Sprintf("No project skill lock found (%s).", lockPath))
			m.appendLine("Use /skills lock to generate a lockfile for project skills.")
			m.refreshViewport()
			return nil
		}

		projectSkills := make([]skill.Skill, 0)
		for _, s := range m.skills.List() {
			if s.Scope == skill.ScopeProject {
				projectSkills = append(projectSkills, s)
			}
		}
		report := skill.VerifyProjectSkills(lock, projectSkills)
		report.LockPath = lockPath
		m.skills.SetProjectLock(lockPath, &report)

		m.appendLine(fmt.Sprintf("Project Skill Lock (%s):", report.LockPath))
		verified, drifted, missing, unlocked := report.Summary()
		m.appendLine(fmt.Sprintf("  Summary: %d verified, %d drifted, %d missing, %d unlocked", verified, drifted, missing, unlocked))
		for _, res := range report.Results {
			switch res.Status {
			case skill.LockStatusVerified:
				m.appendLine(fmt.Sprintf("  [verified] %s (%s)", res.Name, shortHash(res.ComputedHash)))
			case skill.LockStatusDrifted:
				m.appendError(fmt.Sprintf("  [drifted]  %s: expected %s, got %s", res.Name, shortHash(res.ExpectedHash), shortHash(res.ComputedHash)))
			case skill.LockStatusMissing:
				m.appendError(fmt.Sprintf("  [missing]  %s: expected %s (not on disk)", res.Name, shortHash(res.ExpectedHash)))
			case skill.LockStatusUnlocked:
				m.appendLine(fmt.Sprintf("  [unlocked] %s: on disk but not locked", res.Name))
			}
		}
		if report.IsClean() {
			m.appendLine("All locked skills verified cleanly.")
		}
		m.refreshViewport()
		return nil
	}
	if trimmedArg == "lock" {
		projectSkills := make([]skill.Skill, 0)
		for _, s := range m.skills.List() {
			if s.Scope == skill.ScopeProject {
				projectSkills = append(projectSkills, s)
			}
		}
		if len(projectSkills) == 0 {
			m.appendError("No project skills found to lock. Only project-scoped skills can be locked.")
			m.refreshViewport()
			return nil
		}

		lockPath := m.skills.ProjectLockPath()
		if lockPath == "" {
			lockPath = filepath.Join(m.workDir, skill.LockFileName)
		}

		existingLock, _ := skill.ReadLockFile(lockPath)
		newLock, err := skill.GenerateProjectLock(projectSkills, &existingLock)
		if err != nil {
			m.appendError(fmt.Sprintf("generate project skill lock: %v", err))
			m.refreshViewport()
			return nil
		}

		if err := skill.WriteLockFile(lockPath, newLock); err != nil {
			m.appendError(fmt.Sprintf("write project skill lock: %v", err))
			m.refreshViewport()
			return nil
		}

		report := skill.VerifyProjectSkills(newLock, projectSkills)
		report.LockPath = lockPath
		m.skills.SetProjectLock(lockPath, &report)

		m.appendLine(fmt.Sprintf("Locked %d project skill(s) to %s.", len(newLock.Skills), lockPath))
		m.refreshViewport()
		return nil
	}
	if trimmedArg == "toggle" {
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			m.appendError("usage: /skills toggle <name>")
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
		m.persistActiveSkills()
		if view, _ := m.panes.bottom.find(skillsViewID).(*skillsPaneView); view != nil {
			_ = view.refreshItems(newPaneRenderContext(m))
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
			m.appendError(fmt.Sprintf("usage: /skills %s <name>", trimmedArg))
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
		m.persistActiveSkills()
		if view, _ := m.panes.bottom.find(skillsViewID).(*skillsPaneView); view != nil {
			_ = view.refreshItems(newPaneRenderContext(m))
		}
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
		m.appendLine(fmt.Sprintf("[x] Skill %q is already active. Use /skills toggle %s to deactivate.", s.Name, s.Name))
		m.refreshViewport()
		return nil
	}
	if err := m.skills.Activate(s.Name); err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	m.persistActiveSkills()
	if view, _ := m.panes.bottom.find(skillsViewID).(*skillsPaneView); view != nil {
		_ = view.refreshItems(newPaneRenderContext(m))
	}
	m.appendLine(fmt.Sprintf("[x] Activated skill %s [%s]: %s", s.Name, s.Scope, s.Description))
	if len(s.Resources) > 0 {
		if len(s.Resources) <= 5 {
			m.appendLine("Bundled resources:")
			for _, r := range s.Resources {
				m.appendLine("  - " + r)
			}
		} else {
			m.appendLine(fmt.Sprintf("Bundled resources: %s and %d more", strings.Join(s.Resources[:3], ", "), len(s.Resources)-3))
		}
	}
	m.refreshViewport()
	return nil
}

// Tool call commands (/call)

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

func (m *bubbleModel) persistActiveSkills() {
	if m == nil || m.skills == nil {
		return
	}
	active := m.skills.ActivatedList()
	slices.Sort(active)
	// First condition: if have .protonman folder in project save here first
	if appdirs.HasProjectRoot("", m.workDir) {
		if err := m.application.Projects.SaveActiveSkills(m.workDir, active); err != nil {
			m.appendError(fmt.Sprintf("Failed to save project skill list: %v", err))
		}
		return
	}
	// Else save in global
	if err := m.application.UserSettings.SaveActiveSkills(active); err != nil {
		m.appendError(fmt.Sprintf("Failed to save global skill list: %v", err))
	}
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}
