package runtime

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/commandutil"
	projectpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/project"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

type paneRenderContext struct {
	width            int
	height           int
	activeModel      string
	spinner          string
	projectTrusted   bool
	hasWorkDir       bool
	workDir          string
	skillItems       []skillListItem
	todos            []tododomain.Item
	slashMatches     []slashCommand
	projectFacts     []projectpane.ProjectFact
	agentSnapshot    []agent.AgentStatus
	agentActivity    map[string]string
	subagentsEnabled bool
	providers        map[string]config.ProviderConfig
}

func newPaneRenderContext(m *bubbleModel) paneRenderContext {
	ctx := paneRenderContext{width: defaultBubbleWidth, height: defaultBubbleHeight}
	if m == nil {
		return ctx
	}
	ctx.width = m.layout.width
	ctx.height = m.layout.height
	ctx.activeModel = m.activeModel
	ctx.spinner = m.spinner.View()
	ctx.projectTrusted = m.projectTrusted
	ctx.hasWorkDir = m.workDir != ""
	ctx.workDir = m.workDir
	ctx.todos = tododomain.CloneItems(m.todo)
	ctx.slashMatches = append([]slashCommand(nil), m.slashMatches()...)
	ctx.agentSnapshot = append([]agent.AgentStatus(nil), m.agentSnapshot...)
	ctx.subagentsEnabled = m.subagentsEnabled
	ctx.providers = make(map[string]config.ProviderConfig, len(m.providers))
	for name, cfg := range m.providers {
		ctx.providers[name] = cfg
	}
	ctx.agentActivity = make(map[string]string, len(m.agentActivity))
	for id, state := range m.agentActivity {
		ctx.agentActivity[id] = state.String()
	}
	if m.skills != nil {
		for _, item := range m.skills.List() {
			ctx.skillItems = append(ctx.skillItems, skillListItem{name: item.Name, active: m.skills.IsActivated(item.Name)})
		}
	}
	permissionMode := "ask"
	if m.service != nil {
		permissionMode = m.service.Mode().String()
	}
	ctx.projectFacts = []projectpane.ProjectFact{
		{Label: "Model", Value: projectpane.FallbackValue(m.activeModel, "not selected"), Source: string(m.projectSource(config.FieldModelDefault))},
		{Label: "Provider", Value: projectpane.FallbackValue(m.activeProvider, "not selected"), Source: string(m.projectSource(config.FieldModelProvider))},
		{Label: "Agent", Value: projectpane.FallbackValue(m.agentProfile, "universal"), Source: string(m.projectSource(config.FieldAgentProfile))},
		{Label: "Thinking", Value: reasoningEffortLabel(m.reasoningEffort), Source: m.reasoningSourceLabel()},
		{Label: "Subagents", Value: commandutil.SubagentsEnabledLabel(m.subagentsEnabled), Source: string(m.projectSource(config.FieldAgentSubagentsEnabled))},
		{Label: "Permission", Value: permissionMode, Source: string(m.projectSource(config.FieldUIPermissionMode))},
		{Label: "Tool calls", Value: projectpane.FormatLimit(m.maxToolCalls), Source: string(m.projectSource(config.FieldAgentMaxToolCalls))},
	}
	return ctx
}
