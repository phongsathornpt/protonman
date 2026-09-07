package app

import (
	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// Agents owns inbound lifecycle/control access to the subagent coordinator.
type Agents struct{ coordinator *agent.Coordinator }

func NewAgents(coordinator *agent.Coordinator) Agents { return Agents{coordinator: coordinator} }
func (a Agents) Available() bool                      { return a.coordinator != nil }
func (a Agents) Subscribe(buffer int) (<-chan agent.Event, func()) {
	if a.coordinator == nil {
		ch := make(chan agent.Event)
		close(ch)
		return ch, func() {}
	}
	return a.coordinator.Subscribe(buffer)
}
func (a Agents) List() []agent.AgentStatus {
	if a.coordinator == nil {
		return nil
	}
	return a.coordinator.List()
}
func (a Agents) CancelByParent(parentID string) int {
	if a.coordinator == nil {
		return 0
	}
	return a.coordinator.CancelByParent(parentID)
}
func (a Agents) SetPermissionMode(mode permission.Mode) {
	if a.coordinator != nil {
		a.coordinator.SetPermissionMode(mode)
	}
}
func (a Agents) SetReasoningEffort(effort sdk.ReasoningEffort) {
	if a.coordinator != nil {
		a.coordinator.SetReasoningEffort(effort)
	}
}
func (a Agents) SetPrompt(prompt toolcall.PermissionPrompt) {
	if a.coordinator != nil {
		a.coordinator.SetPrompt(prompt)
	}
}
func (a Agents) SetCallGuard(guard toolcall.CallGuard) {
	if a.coordinator != nil {
		a.coordinator.SetCallGuard(guard)
	}
}

func (a Agents) Coordinator() *agent.Coordinator { return a.coordinator }
