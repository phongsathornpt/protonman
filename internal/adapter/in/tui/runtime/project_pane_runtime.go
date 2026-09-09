package runtime

import (
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/commandutil"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/projectpolicy"
	projectpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/project"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
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
	facts := []projectpane.ProjectFact{{Label: "Model", Value: projectpolicy.FallbackValue(m.activeModel, "not selected"), Source: string(m.projectSource(config.FieldModelDefault))}, {Label: "Provider", Value: projectpolicy.FallbackValue(m.activeProvider, "not selected"), Source: string(m.projectSource(config.FieldModelProvider))}, {Label: "Agent", Value: projectpolicy.FallbackValue(m.agentProfile, "universal"), Source: string(m.projectSource(config.FieldAgentProfile))}, {Label: "Thinking", Value: reasoningEffortLabel(m.reasoningEffort), Source: string(m.projectSource(config.FieldAgentReasoningEffort))}, {Label: "Subagents", Value: commandutil.SubagentsEnabledLabel(m.subagentsEnabled), Source: string(m.projectSource(config.FieldAgentSubagentsEnabled))}, {Label: "Permission", Value: m.service.Mode().String(), Source: string(m.projectSource(config.FieldUIPermissionMode))}, {Label: "Tool calls", Value: projectpolicy.FormatLimit(m.maxToolCalls), Source: string(m.projectSource(config.FieldAgentMaxToolCalls))}}
	errorText := ""
	if v.err != nil {
		errorText = v.err.Error()
	}
	rows := projectpane.ProjectRows(projectpane.ProjectSnapshot{Width: m.width, Height: m.height, WorkDir: m.workDir, RootName: appdirs.RootDirName, ConfigName: appdirs.ConfigFileName, TrustEnv: envconfig.TrustProject, Loading: v.loading, ErrorText: errorText, Exists: state.Exists, ConfigExists: state.ConfigExists, ConfigLoaded: state.ConfigLoaded, Trusted: state.Trusted, SkillsExists: state.SkillsExists, SkillCount: state.SkillCount, Facts: facts, Notice: v.notice})
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
	m.requestRelayout()
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
		m.requestRelayout()
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
	m.requestRelayout()
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
	m.requestRelayout()
	return m, nil
}

func (m *bubbleModel) projectSource(field string) config.ValueSource {
	if m == nil {
		return config.SourceDefault
	}
	return projectpolicy.Source(m.projectConfigProvenance, field)
}
