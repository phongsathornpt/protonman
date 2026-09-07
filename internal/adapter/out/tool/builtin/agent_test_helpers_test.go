package builtin

import (
	"github.com/projectTHORN/proton/internal/adapter/out/tool/agent"
	"github.com/projectTHORN/proton/internal/feature/agent"
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
