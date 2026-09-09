package runtime

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"strings"
)

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
