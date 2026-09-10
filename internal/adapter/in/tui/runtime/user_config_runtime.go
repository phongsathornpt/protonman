package runtime

import (
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/commandutil"
	"strconv"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

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
		enabled, err := commandutil.ParseSubagentsEnabled(fields[2])
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
		if err := m.validateReasoningEffort(effort); err != nil {
			m.appendError(err.Error())
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
	rule, err := commandutil.ParsePermissionRuleArgs(args)
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
	rule, err := commandutil.ParsePermissionRuleArgs(parts)
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
		m.applyReasoningPreference(effort, reasoningPreferenceConfig)
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
