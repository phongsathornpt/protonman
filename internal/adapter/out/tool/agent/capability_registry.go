package agenttool

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

// CapabilityRegistry filters the primary agent tool surface according to the
// live subagent capability while preserving lifecycle control for existing
// agents after new delegation has been disabled.
type CapabilityRegistry struct {
	base        tool.Registry
	coordinator *agent.Coordinator
}

var _ tool.SnapshotRegistry = (*CapabilityRegistry)(nil)

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

func (r *CapabilityRegistry) LookupSnapshot(name string) (tool.HandlerSnapshot, bool) {
	if r == nil || r.base == nil {
		return tool.HandlerSnapshot{}, false
	}
	enabled, hasAgents := r.capabilityState()
	if !visibleSubagentTool(name, enabled, hasAgents) {
		return tool.HandlerSnapshot{}, false
	}
	snapshot, ok := r.baseSnapshot(name)
	if !ok {
		return tool.HandlerSnapshot{}, false
	}
	if strings.TrimSpace(name) == tool.NameSubagent && !enabled {
		snapshot.Definition = lifecycleOnlySubagentDefinition(snapshot.Definition)
		snapshot.InputValidator, snapshot.OutputValidator, snapshot.ValidatorsCompiled = compileSnapshotValidators(snapshot.Definition)
	}
	return snapshot, true
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

func (r *CapabilityRegistry) CompiledValidators(name string) (input, output *usecase.ToolSchemaValidator, ok bool) {
	snapshot, ok := r.LookupSnapshot(name)
	if !ok || !snapshot.ValidatorsCompiled {
		return nil, nil, false
	}
	return snapshot.InputValidator, snapshot.OutputValidator, true
}

func (r *CapabilityRegistry) baseSnapshot(name string) (tool.HandlerSnapshot, bool) {
	if snapshots, ok := r.base.(tool.SnapshotRegistry); ok {
		return snapshots.LookupSnapshot(name)
	}
	handler, ok := r.base.Lookup(name)
	if !ok {
		return tool.HandlerSnapshot{}, false
	}
	definition := handler.Definition()
	input, output, compiled := compileSnapshotValidators(definition)
	return tool.HandlerSnapshot{
		Handler: handler, Definition: definition,
		InputValidator: input, OutputValidator: output,
		ValidatorsCompiled: compiled,
	}, true
}

func compileSnapshotValidators(definition tool.Definition) (input, output *usecase.ToolSchemaValidator, ok bool) {
	sdkTool := domain.Tool{
		Name: definition.Name, Description: definition.Description,
		InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema,
	}
	input, err := usecase.CompileToolInputValidator(sdkTool)
	if err != nil {
		return nil, nil, false
	}
	output, err = usecase.CompileToolOutputValidator(sdkTool)
	if err != nil {
		return nil, nil, false
	}
	return input, output, true
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
	def.Description = "Lifecycle controls for existing subagents. Available actions in this request: wait, get, list, and cancel."
	input := make(map[string]any, len(def.InputSchema))
	for key, value := range def.InputSchema {
		input[key] = value
	}
	properties, _ := def.InputSchema["properties"].(map[string]any)
	nextProperties := map[string]any{
		"action": map[string]any{
			"type": "string", "enum": []string{tool.ActionWait, tool.ActionGet, tool.ActionList, tool.ActionCancel},
			"description": "Lifecycle operation for an existing subagent.",
		},
	}
	if timeout, ok := properties["timeoutSeconds"].(map[string]any); ok {
		nextTimeout := make(map[string]any, len(timeout)+1)
		for key, value := range timeout {
			nextTimeout[key] = value
		}
		nextTimeout["description"] = "Optional bounded timeout for action=wait."
		nextProperties["timeoutSeconds"] = nextTimeout
	}
	if agentID, ok := properties["agentId"].(map[string]any); ok {
		nextAgentID := make(map[string]any, len(agentID)+1)
		for key, value := range agentID {
			nextAgentID[key] = value
		}
		nextAgentID["description"] = "Target existing subagent for action=get or action=cancel."
		nextProperties["agentId"] = nextAgentID
	}
	input["properties"] = nextProperties
	input["required"] = []string{"action"}
	def.InputSchema = input
	return def
}
