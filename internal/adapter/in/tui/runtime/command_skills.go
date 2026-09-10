package runtime

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

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
