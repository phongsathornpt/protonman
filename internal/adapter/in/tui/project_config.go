package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/phongsathornpt/proton/internal/adapter/out/config"
	"github.com/phongsathornpt/proton/internal/app"
	"github.com/phongsathornpt/proton/internal/core/permission"
	"github.com/phongsathornpt/proton/internal/feature/agent"
	sdk "github.com/phongsathornpt/proton/proton-sdk"
)

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
