package agenttool

import (
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"strings"
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
	filtered := &CapabilityRegistry{base: base, coordinator: coordinator}
	if dynamic, ok := base.(tool.DynamicRegistrar); ok {
		return &dynamicCapabilityRegistry{CapabilityRegistry: filtered, dynamic: dynamic}
	}
	return filtered
}

type dynamicCapabilityRegistry struct {
	*CapabilityRegistry
	dynamic tool.DynamicRegistrar
}

func (r *dynamicCapabilityRegistry) Register(handler tool.Handler) error {
	return r.dynamic.Register(handler)
}

func (r *dynamicCapabilityRegistry) RegisterBatch(handlers []tool.Handler) error {
	return r.dynamic.RegisterBatch(handlers)
}

func (r *dynamicCapabilityRegistry) ReplaceNamespace(prefix string, handlers []tool.Handler) error {
	return r.dynamic.ReplaceNamespace(prefix, handlers)
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
	enabled, hasAgents := r.capabilityState()
	for _, def := range base {
		if !visibleSubagentTool(def.Name, enabled, hasAgents) {
			continue
		}
		if strings.TrimSpace(def.Name) == tool.NameSubagent && !enabled {
			def = lifecycleOnlySubagentDefinition(def)
		}
		out = append(out, def)
	}
	return out
}

func (r *CapabilityRegistry) CompiledValidators(name string) (input, output *sdk.ToolSchemaValidator, ok bool) {
	if r == nil || r.base == nil || !r.visible(name) {
		return nil, nil, false
	}
	type compiledRegistry interface {
		CompiledValidators(string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool)
	}
	compiled, ok := r.base.(compiledRegistry)
	if !ok {
		return nil, nil, false
	}
	return compiled.CompiledValidators(name)
}

func (r *CapabilityRegistry) visible(name string) bool {
	enabled, hasAgents := r.capabilityState()
	return visibleSubagentTool(name, enabled, hasAgents)
}

func (r *CapabilityRegistry) capabilityState() (enabled, hasAgents bool) {
	if r == nil || r.coordinator == nil {
		return false, false
	}
	enabled = r.coordinator.Enabled()
	if !enabled {
		hasAgents = r.coordinator.HasAgents()
	}
	return enabled, hasAgents
}

func visibleSubagentTool(name string, enabled, hasAgents bool) bool {
	if strings.TrimSpace(name) != tool.NameSubagent {
		return true
	}
	return enabled || hasAgents
}

func lifecycleOnlySubagentDefinition(def tool.Definition) tool.Definition {
	input := make(map[string]any, len(def.InputSchema))
	for key, value := range def.InputSchema {
		input[key] = value
	}
	properties, _ := def.InputSchema["properties"].(map[string]any)
	nextProperties := make(map[string]any, len(properties))
	for key, value := range properties {
		nextProperties[key] = value
	}
	action, _ := properties["action"].(map[string]any)
	nextAction := make(map[string]any, len(action))
	for key, value := range action {
		nextAction[key] = value
	}
	nextAction["enum"] = []string{"wait", "get", "list", "cancel"}
	nextProperties["action"] = nextAction
	input["properties"] = nextProperties
	def.InputSchema = input
	return def
}
