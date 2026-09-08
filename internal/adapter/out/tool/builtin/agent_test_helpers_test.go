package builtin

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/agent"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func withAgentTools(coordinator *agent.Coordinator) RegistryOption {
	return WithAdditionalHandlers(agenttool.NewSubagent(coordinator))
}
