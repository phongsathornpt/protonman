package builtin

import (
	"github.com/phongsathornpt/proton/internal/adapter/out/tool/agent"
	"github.com/phongsathornpt/proton/internal/feature/agent"
)

func withAgentTools(coordinator *agent.Coordinator) RegistryOption {
	return WithAdditionalHandlers(
		agenttool.NewDelegateTask(coordinator),
		agenttool.NewWaitAgent(coordinator),
		agenttool.NewGetAgent(coordinator),
		agenttool.NewListAgents(coordinator),
		agenttool.NewCancelAgent(coordinator),
	)
}
