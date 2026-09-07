package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/appdirs"
	projectdomain "github.com/projectTHORN/proton/internal/project"
)

const projectViewID = "project"

type projectLoadedMsg struct {
	requestID uint64
	state     projectdomain.State
	err       error
}

type projectPaneView struct {
	requestID uint64
	loading   bool
	state     projectdomain.State
	err       error
}

func (*projectPaneView) ID() string             { return projectViewID }
func (*projectPaneView) ReplacesComposer() bool { return true }

func (v *projectPaneView) Render(m *bubbleModel) string {
	rows := []string{brandStyle.Render("Project Settings")}
	root := strings.TrimSpace(m.workDir)
	if root == "" {
		root = "."
	}
	rows = append(rows, mutedStyle.Render(truncateWithEllipsis(root, maxInt(12, m.width-10))), "")
	if v.loading {
		rows = append(rows, mutedStyle.Render("Loading "+appdirs.RootDirName+" workspace state..."), "", mutedStyle.Render("esc close"))
		return renderModalRows(m, accentAssistant, rows)
	}
	if v.err != nil {
		rows = append(rows,
			errorStyle.Render("Failed to inspect "+appdirs.RootDirName),
			mutedStyle.Render(truncateWithEllipsis(v.err.Error(), maxInt(12, m.width-10))),
			"",
			mutedStyle.Render("r retry · esc close"),
		)
		return renderModalRows(m, accentAssistant, rows)
	}

	state := v.state
	protonStatus := "not found"
	if state.Exists {
		protonStatus = "detected"
	}
	rows = append(rows, projectFact(appdirs.RootDirName, protonStatus))

	configStatus := "not found"
	switch {
	case state.ConfigLoaded:
		configStatus = "loaded · trusted"
	case state.ConfigExists && !state.Trusted:
		configStatus = "ignored · untrusted"
	case state.ConfigExists:
		configStatus = "detected"
	}
	rows = append(rows, projectFact("Config", configStatus))

	skillsStatus := "not found"
	if state.SkillsExists {
		skillsStatus = fmt.Sprintf("%d detected", state.SkillCount)
		if !state.Trusted && state.SkillCount > 0 {
			skillsStatus += " · inactive until trusted"
		}
	}
	rows = append(rows, projectFact("Skills", skillsStatus), "")

	rows = append(rows,
		projectFact("Model", fallbackProjectValue(m.activeModel, "not selected")),
		projectFact("Provider", fallbackProjectValue(m.activeProvider, "not selected")),
		projectFact("Agent", fallbackProjectValue(m.agentProfile, "default")),
		projectFact("Thinking", reasoningEffortLabel(m.reasoningEffort)),
		projectFact("Permission", m.service.Mode().String()),
		projectFact("Rounds", formatProjectLimit(m.maxRounds)),
		"",
	)
	if state.ConfigExists && !state.Trusted {
		rows = append(rows, warningStyle.Render("Project config and skills are present but not trusted."))
	} else if !state.Exists {
		rows = append(rows, mutedStyle.Render("No project-local Proton settings are configured."))
	}
	rows = append(rows, mutedStyle.Render("r reload · esc close"))
	if layoutModeForHeight(m.height) == layoutTiny {
		rows = compactPickerRows(rows)
	}
	return renderModalRows(m, accentAssistant, rows)
}

func (v *projectPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
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
		state, err := projectdomain.Discover(m.ctx, projectdomain.Options{
			WorkDir:       m.workDir,
			Trusted:       m.projectTrusted,
			ConfigSources: sources,
		})
		return projectLoadedMsg{requestID: requestID, state: state, err: err}
	}
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
	return fmt.Sprintf("%-12s %s", label, value)
}

func fallbackProjectValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatProjectLimit(value int) string {
	if value <= 0 {
		return "unbounded"
	}
	return fmt.Sprintf("%d", value)
}
