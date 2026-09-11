package runtime

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

type paneRenderContext struct {
	width              int
	height             int
	activeModel        string
	spinner            string
	projectTrusted     bool
	hasWorkDir         bool
	workDir            string
	skillItems         []skillListItem
	todos              []tododomain.Item
	slashMatches       []slashCommand
	agentSnapshot      []agent.AgentStatus
	agentActivity      map[string]string
	subagentsEnabled   bool
	keyboardCapability keyboardCapability
	providers          map[string]config.ProviderConfig
}

func newPaneRenderContext(m *bubbleModel) paneRenderContext {
	ctx := paneRenderContext{width: defaultBubbleWidth, height: defaultBubbleHeight}
	if m == nil {
		return ctx
	}
	ctx.width = m.layout.width
	ctx.height = m.layout.height
	ctx.activeModel = m.activeModel
	ctx.spinner = m.spinnerIndicator()
	ctx.projectTrusted = m.projectTrusted
	ctx.hasWorkDir = m.workDir != ""
	ctx.workDir = m.workDir
	ctx.todos = tododomain.CloneItems(m.todo)
	ctx.slashMatches = append([]slashCommand(nil), m.slashMatches()...)
	ctx.agentSnapshot = append([]agent.AgentStatus(nil), m.agentSnapshot...)
	ctx.subagentsEnabled = m.subagentsEnabled
	ctx.keyboardCapability = m.keyboardCapability
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

	return ctx
}
