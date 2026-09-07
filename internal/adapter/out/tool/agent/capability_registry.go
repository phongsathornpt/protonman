package agenttool

import (
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/feature/agent"
)

// CapabilityRegistry filters the primary agent tool surface according to the
// live subagent capability while preserving lifecycle control for existing
// agents after new delegation has been disabled.
type CapabilityRegistry struct {
	base        tool.Registry
	coordinator *agent.Coordinator
}

func NewCapabilityRegistry(base tool.Registry, coordinator *agent.Coordinator) tool.Registry {
	if base == nil {
		return nil
	}
	return &CapabilityRegistry{base: base, coordinator: coordinator}
}

func (r *CapabilityRegistry) Lookup(name string) (tool.Handler, bool) {
	if r == nil || r.base == nil || !r.visible(name) {
		return nil, false
	}
	return r.base.Lookup(name)
}

func (r *CapabilityRegistry) Definitions() []tool.Definition {
	if r == nil || r.base == nil {
		return nil
	}
	base := r.base.Definitions()
	out := make([]tool.Definition, 0, len(base))
	for _, def := range base {
		if r.visible(def.Name) {
			out = append(out, def)
		}
	}
	return out
}

func (r *CapabilityRegistry) visible(name string) bool {
	if !isSubagentTool(name) {
		return true
	}
	if r.coordinator == nil {
		return false
	}
	if name == "delegate_task" {
		return r.coordinator.Enabled()
	}
	return r.coordinator.Enabled() || len(r.coordinator.List()) > 0
}

func isSubagentTool(name string) bool {
	switch name {
	case "delegate_task", "wait_agent", "get_agent", "list_agents", "cancel_agent":
		return true
	default:
		return false
	}
}
