package runtime

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"strconv"
	"strings"
)

const projectViewID = "project"

type projectInitializedMsg struct {
	result app.ProjectInitResult
	err    error
}

type projectLoadedMsg struct {
	requestID uint64
	state     app.ProjectState
	err       error
}

type projectPaneView struct {
	requestID uint64
	loading   bool
	state     app.ProjectState
	err       error
	notice    string
}

func (*projectPaneView) ID() string {
	return projectViewID
}

func (*projectPaneView) ReplacesComposer() bool {
	return true
}

func (v *projectPaneView) Render(m *bubbleModel) string {
	state := v.state
	facts := []pane.ProjectFact{{Label: "Model", Value: fallbackProjectValue(m.activeModel, "not selected"), Source: string(m.projectSource(config.FieldModelDefault))}, {Label: "Provider", Value: fallbackProjectValue(m.activeProvider, "not selected"), Source: string(m.projectSource(config.FieldModelProvider))}, {Label: "Agent", Value: fallbackProjectValue(m.agentProfile, "universal"), Source: string(m.projectSource(config.FieldAgentProfile))}, {Label: "Thinking", Value: reasoningEffortLabel(m.reasoningEffort), Source: string(m.projectSource(config.FieldAgentReasoningEffort))}, {Label: "Subagents", Value: subagentsEnabledLabel(m.subagentsEnabled), Source: string(m.projectSource(config.FieldAgentSubagentsEnabled))}, {Label: "Permission", Value: m.service.Mode().String(), Source: string(m.projectSource(config.FieldUIPermissionMode))}, {Label: "Tool calls", Value: formatProjectLimit(m.maxToolCalls), Source: string(m.projectSource(config.FieldAgentMaxToolCalls))}}
	errorText := ""
	if v.err != nil {
		errorText = v.err.Error()
	}
	rows := pane.ProjectRows(pane.ProjectSnapshot{Width: m.width, Height: m.height, WorkDir: m.workDir, RootName: appdirs.RootDirName, ConfigName: appdirs.ConfigFileName, TrustEnv: envconfig.TrustProject, Loading: v.loading, ErrorText: errorText, Exists: state.Exists, ConfigExists: state.ConfigExists, ConfigLoaded: state.ConfigLoaded, Trusted: state.Trusted, SkillsExists: state.SkillsExists, SkillCount: state.SkillCount, Facts: facts, Notice: v.notice})
	return renderModalRows(m, accentAssistant, rows)
}

func (v *projectPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "r":
		return true, v.reload(m)
	case "esc", "q":
		m.bottom.remove(projectViewID)
		return true, nil
	default:
		return false, nil
	}
}

func (v *projectPaneView) reload(m *bubbleModel) tea.Cmd {
	v.requestID++
	v.loading = true
	v.err = nil
	requestID := v.requestID
	sources := append([]string(nil), m.projectConfigSources...)
	return func() tea.Msg {
		state, err := (app.Projects{}).Discover(m.ctx, app.ProjectDiscoveryOptions{WorkDir: m.workDir, Trusted: m.projectTrusted, ConfigSources: sources})
		return projectLoadedMsg{requestID: requestID, state: state, err: err}
	}
}

func (m *bubbleModel) initProject() tea.Cmd {
	view, _ := m.bottom.find(projectViewID).(*projectPaneView)
	if view == nil {
		view = &projectPaneView{}
		m.bottom.push(view)
	}
	view.loading = true
	view.err = nil
	view.notice = ""
	m.relayout()
	return func() tea.Msg {
		result, err := (app.Projects{}).Init(m.ctx, m.workDir)
		return projectInitializedMsg{result: result, err: err}
	}
}

func (m *bubbleModel) updateProjectInitialized(message projectInitializedMsg) (tea.Model, tea.Cmd) {
	view, _ := m.bottom.find(projectViewID).(*projectPaneView)
	if view == nil {
		return m, nil
	}
	if message.err != nil {
		view.loading = false
		view.err = message.err
		m.relayout()
		return m, nil
	}
	if message.result.Created {
		view.notice = "Created " + appdirs.RootDirName + "/" + appdirs.ConfigFileName
	} else {
		view.notice = appdirs.RootDirName + "/" + appdirs.ConfigFileName + " already exists"
	}
	return m, view.reload(m)
}

func (m *bubbleModel) openProjectPane() tea.Cmd {
	view, _ := m.bottom.find(projectViewID).(*projectPaneView)
	if view == nil {
		view = &projectPaneView{}
		m.bottom.push(view)
	}
	m.relayout()
	return view.reload(m)
}

func (m *bubbleModel) updateProjectLoaded(message projectLoadedMsg) (tea.Model, tea.Cmd) {
	view, _ := m.bottom.find(projectViewID).(*projectPaneView)
	if view == nil || message.requestID != view.requestID {
		return m, nil
	}
	view.loading = false
	view.err = message.err
	if message.err == nil {
		view.state = message.state
	}
	m.relayout()
	return m, nil
}

func projectFact(label, value string) string {
	return pane.ProjectFactLine(label, value)
}

func fallbackProjectValue(value, fallback string) string {
	return pane.FallbackValue(value, fallback)
}

func formatProjectLimit(value int) string {
	return pane.FormatLimit(value)
}

func cloneProjectProvenance(in map[string]config.ValueSource) map[string]config.ValueSource {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]config.ValueSource, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m *bubbleModel) projectSource(field string) config.ValueSource {
	if m == nil || m.projectConfigProvenance == nil {
		return config.SourceDefault
	}
	if source, ok := m.projectConfigProvenance[field]; ok {
		return source
	}
	return config.SourceDefault
}

type projectSettingSavedMsg struct {
	field string
	value any
	err   error
}

func (m *bubbleModel) handleProjectSet(argument string) tea.Cmd {
	if !m.projectTrusted {
		m.appendError("project settings are read-only until the workspace is trusted")
		m.refreshViewport()
		return nil
	}
	parts := strings.Fields(strings.TrimSpace(argument))
	if len(parts) < 2 {
		m.appendError("usage: /project set <agent|thinking|subagents|tool-calls|permission> <value>")
		m.refreshViewport()
		return nil
	}
	field := strings.ToLower(parts[0])
	value := strings.Join(parts[1:], " ")
	switch field {
	case "agent", "profile":
		profile, err := agent.ParseProfile(value)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		return saveProjectAgentCmd(m.workDir, string(profile))
	case "thinking", "reasoning":
		effort, err := sdk.ParseReasoningEffort(value)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		if effort != sdk.ReasoningDefault {
			if _, err := m.activeResolvedModelProfile().ResolveExplicitReasoning(effort); err != nil {
				m.appendError(err.Error())
				m.refreshViewport()
				return nil
			}
		}
		return saveProjectReasoningCmd(m.workDir, effort)
	case "subagents":
		enabled, err := parseSubagentsEnabled(value)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		return saveProjectSubagentsCmd(m.workDir, enabled)
	case "tool-calls", "tools-limit":
		calls, err := strconv.Atoi(value)
		if err != nil || calls < 0 {
			m.appendError("project tool-calls must be a non-negative integer")
			m.refreshViewport()
			return nil
		}
		return saveProjectToolCallsCmd(m.workDir, calls)
	case "permission", "mode":
		if len(parts) >= 3 && isPermissionAction(parts[1]) {
			return m.handleProjectPermission(strings.Join(parts[1:], " "))
		}
		mode, err := permission.ParseMode(value)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		return saveProjectPermissionCmd(m.workDir, mode)
	default:
		m.appendError(fmt.Sprintf("unknown project setting %q", field))
		m.refreshViewport()
		return nil
	}
}

func saveProjectAgentCmd(workDir, profile string) tea.Cmd {
	return func() tea.Msg {
		err := (app.Projects{}).SaveAgentProfile(workDir, profile)
		return projectSettingSavedMsg{field: config.FieldAgentProfile, value: profile, err: err}
	}
}

func saveProjectSubagentsCmd(workDir string, enabled bool) tea.Cmd {
	return func() tea.Msg {
		err := (app.Projects{}).SaveSubagentsEnabled(workDir, enabled)
		return projectSettingSavedMsg{field: config.FieldAgentSubagentsEnabled, value: enabled, err: err}
	}
}

func saveProjectReasoningCmd(workDir string, effort sdk.ReasoningEffort) tea.Cmd {
	return func() tea.Msg {
		err := (app.Projects{}).SaveReasoningEffort(workDir, effort)
		return projectSettingSavedMsg{field: config.FieldAgentReasoningEffort, value: effort, err: err}
	}
}

func saveProjectToolCallsCmd(workDir string, calls int) tea.Cmd {
	return func() tea.Msg {
		err := (app.Projects{}).SaveMaxToolCalls(workDir, calls)
		return projectSettingSavedMsg{field: config.FieldAgentMaxToolCalls, value: calls, err: err}
	}
}

func saveProjectPermissionCmd(workDir string, mode permission.Mode) tea.Cmd {
	return func() tea.Msg {
		err := (app.Projects{}).SavePermissionMode(workDir, mode)
		return projectSettingSavedMsg{field: config.FieldUIPermissionMode, value: mode, err: err}
	}
}

func (m *bubbleModel) updateProjectSettingSaved(message projectSettingSavedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendError("failed to save project setting: " + message.err.Error())
		m.refreshViewport()
		return m, nil
	}
	if m.projectConfigProvenance == nil {
		m.projectConfigProvenance = make(map[string]config.ValueSource)
	}
	m.projectConfigProvenance[message.field] = config.SourceProject
	switch message.field {
	case config.FieldAgentProfile:
		m.agentProfile = message.value.(string)
		m.reconfigureRunner()
	case config.FieldAgentSubagentsEnabled:
		m.subagentsEnabled = message.value.(bool)
		m.agents.SetEnabled(m.subagentsEnabled)
		m.reconfigureRunner()
	case config.FieldAgentReasoningEffort:
		m.reasoningEffort = message.value.(sdk.ReasoningEffort)
		m.agents.SetReasoningEffort(m.reasoningEffort)
		m.reconfigureRunner()
	case config.FieldAgentMaxToolCalls:
		m.maxToolCalls = message.value.(int)
		m.reconfigureRunner()
	case config.FieldUIPermissionMode:
		if err := m.setPermissionMode(message.value.(permission.Mode)); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return m, nil
		}
	}
	m.appendLine(successStyle.Render("Project setting saved."))
	m.refreshViewport()
	if view, _ := m.bottom.find(projectViewID).(*projectPaneView); view != nil {
		view.notice = "Project setting saved"
		return m, view.reload(m)
	}
	return m, nil
}

type userSettingSavedMsg struct {
	field string
	value any
	err   error
}

func (m *bubbleModel) executeUserConfigCommand(line, rawName string) tea.Cmd {
	cmdLine := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	cmdLine = strings.TrimSpace(strings.TrimPrefix(cmdLine, rawName))
	fields := strings.Fields(cmdLine)
	if len(fields) >= 1 && strings.ToLower(fields[0]) == "permission" {
		return m.handleUserConfigPermission(fields[1:])
	}
	if len(fields) >= 2 && strings.ToLower(fields[0]) == "set" && strings.ToLower(fields[1]) == "permission" {
		return m.handleUserConfigPermission(fields[2:])
	}
	if len(fields) != 3 || strings.ToLower(fields[0]) != "set" {
		m.appendError("usage: /config set <subagents|thinking|tool-calls> <value> or /config permission <allow|deny|ask> <tool> [pattern]")
		m.refreshViewport()
		return nil
	}
	switch strings.ToLower(fields[1]) {
	case "subagents":
		enabled, err := parseSubagentsEnabled(fields[2])
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		return func() tea.Msg {
			err := (app.UserSettings{}).SaveSubagentsEnabled(enabled)
			return userSettingSavedMsg{field: config.FieldAgentSubagentsEnabled, value: enabled, err: err}
		}
	case "thinking", "reasoning":
		effort, err := sdk.ParseReasoningEffort(fields[2])
		if err != nil {
			m.appendError("invalid reasoning effort: use auto, none, low, medium, high, xhigh, or max")
			m.refreshViewport()
			return nil
		}
		return func() tea.Msg {
			err := (app.UserSettings{}).SaveReasoningEffort(effort)
			return userSettingSavedMsg{field: config.FieldAgentReasoningEffort, value: effort, err: err}
		}
	case "tool-calls", "tool_calls", "toolcalls":
		limit, err := strconv.Atoi(fields[2])
		if err != nil || limit < 0 {
			m.appendError("invalid tool-calls limit: must be a non-negative integer")
			m.refreshViewport()
			return nil
		}
		return func() tea.Msg {
			err := (app.UserSettings{}).SaveMaxToolCalls(limit)
			return userSettingSavedMsg{field: config.FieldAgentMaxToolCalls, value: limit, err: err}
		}
	default:
		m.appendError("usage: /config set <subagents|thinking|tool-calls> <value> or /config permission <allow|deny|ask> <tool> [pattern]")
		m.refreshViewport()
		return nil
	}
}

func (m *bubbleModel) handleUserConfigPermission(args []string) tea.Cmd {
	rule, err := parsePermissionRuleArgs(args)
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	if m.service != nil {
		_ = m.service.AddRule(rule)
	}
	return func() tea.Msg {
		err := (app.UserSettings{}).SavePermissionRule(rule)
		return permissionRuleSavedMsg{scope: "user", rule: rule, err: err}
	}
}

func (m *bubbleModel) handleProjectPermission(argument string) tea.Cmd {
	if !m.projectTrusted {
		m.appendError("project settings are read-only until the workspace is trusted")
		m.refreshViewport()
		return nil
	}
	parts := strings.Fields(strings.TrimSpace(argument))
	rule, err := parsePermissionRuleArgs(parts)
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	if m.service != nil {
		_ = m.service.AddRule(rule)
	}
	workDir := m.workDir
	return func() tea.Msg {
		err := (app.Projects{}).SavePermissionRule(workDir, rule)
		return permissionRuleSavedMsg{scope: "project", rule: rule, err: err}
	}
}

func parsePermissionRuleArgs(args []string) (permission.Rule, error) {
	if len(args) < 2 {
		return permission.Rule{}, errors.New("usage: permission <allow|deny|ask> <tool> [pattern]")
	}
	action, err := permission.ParseAction(args[0])
	if err != nil {
		return permission.Rule{}, fmt.Errorf("invalid permission action %q: use allow, deny, or ask", args[0])
	}
	toolKind, err := permission.ParseToolKind(args[1])
	if err != nil {
		return permission.Rule{}, err
	}
	patternMode := permission.PatternModeGlob
	if toolKind == permission.ToolWeb {
		patternMode = permission.PatternModeDomain
	}
	pattern := "*"
	if len(args) > 2 {
		pattern = strings.Join(args[2:], " ")
	}
	pattern = permission.NormalizePattern(toolKind, patternMode, pattern)
	return permission.Rule{
		Action:      action,
		Tool:        toolKind,
		Pattern:     pattern,
		PatternMode: patternMode,
	}, nil
}

func isPermissionAction(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow", "deny", "ask":
		return true
	default:
		return false
	}
}

func (m *bubbleModel) updateUserSettingSaved(message userSettingSavedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendError("failed to save user setting: " + message.err.Error())
		m.refreshViewport()
		return m, nil
	}
	switch message.field {
	case config.FieldAgentSubagentsEnabled:
		enabled := message.value.(bool)
		if m.projectSource(config.FieldAgentSubagentsEnabled) == config.SourceProject {
			m.appendLine(successStyle.Render("User subagent default saved."))
			m.appendMuted("The trusted project override remains effective in this workspace.")
			m.refreshViewport()
			return m, nil
		}
		m.subagentsEnabled = enabled
		m.agents.SetEnabled(enabled)
		if m.projectConfigProvenance == nil {
			m.projectConfigProvenance = make(map[string]config.ValueSource)
		}
		m.projectConfigProvenance[config.FieldAgentSubagentsEnabled] = config.SourceUser
		m.reconfigureRunner()
		m.appendLine(successStyle.Render("User subagent default saved and applied."))
		m.refreshViewport()
		return m, nil

	case config.FieldAgentReasoningEffort:
		effort := message.value.(sdk.ReasoningEffort)
		if m.projectSource(config.FieldAgentReasoningEffort) == config.SourceProject {
			m.appendLine(successStyle.Render("User thinking default saved to " + appdirs.UserConfigDisplay() + "."))
			m.appendMuted("The trusted project override remains effective in this workspace.")
			m.refreshViewport()
			return m, nil
		}
		m.reasoningEffort = effort
		m.agents.SetReasoningEffort(effort)
		if m.projectConfigProvenance == nil {
			m.projectConfigProvenance = make(map[string]config.ValueSource)
		}
		m.projectConfigProvenance[config.FieldAgentReasoningEffort] = config.SourceUser
		m.reconfigureRunner()
		m.appendLine(successStyle.Render("User thinking default saved and applied to " + appdirs.UserConfigDisplay() + "."))
		m.refreshViewport()
		return m, nil

	case config.FieldAgentMaxToolCalls:
		limit := message.value.(int)
		if m.projectSource(config.FieldAgentMaxToolCalls) == config.SourceProject {
			m.appendLine(successStyle.Render("User tool call limit saved to " + appdirs.UserConfigDisplay() + "."))
			m.appendMuted("The trusted project override remains effective in this workspace.")
			m.refreshViewport()
			return m, nil
		}
		m.maxToolCalls = limit
		if m.projectConfigProvenance == nil {
			m.projectConfigProvenance = make(map[string]config.ValueSource)
		}
		m.projectConfigProvenance[config.FieldAgentMaxToolCalls] = config.SourceUser
		m.reconfigureRunner()
		m.appendLine(successStyle.Render("User tool call limit saved and applied to " + appdirs.UserConfigDisplay() + "."))
		m.refreshViewport()
		return m, nil

	default:
		m.appendError("unsupported user setting")
		m.refreshViewport()
		return m, nil
	}
}
