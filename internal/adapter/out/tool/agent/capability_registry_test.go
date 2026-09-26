package agenttool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

type capabilityTestRegistry struct{ handlers map[string]tool.Handler }

func (r capabilityTestRegistry) Lookup(name string) (tool.Handler, bool) {
	h, ok := r.handlers[name]
	return h, ok
}
func (r capabilityTestRegistry) Definitions() []tool.Definition {
	defs := make([]tool.Definition, 0, len(r.handlers))
	for _, name := range []string{"subagent"} {
		if h, ok := r.handlers[name]; ok {
			defs = append(defs, h.Definition())
		}
	}
	return defs
}

func TestCapabilityRegistryHidesSubagentToolsWhenDisabledAndIdle(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil, agent.WithEnabled(false))
	defer coord.Close()
	base := capabilityTestRegistry{handlers: map[string]tool.Handler{
		"subagent": NewSubagent(coord),
	}}
	reg := NewCapabilityRegistry(base, coord)
	if got := reg.Definitions(); len(got) != 0 {
		t.Fatalf("definitions = %d, want 0", len(got))
	}
}

func TestCapabilityRegistryKeepsLifecycleToolsForExistingAgents(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileAgility, Task: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	coord.SetEnabled(false)
	base := capabilityTestRegistry{handlers: map[string]tool.Handler{
		"subagent": NewSubagent(coord),
	}}
	reg := NewCapabilityRegistry(base, coord)
	if _, ok := reg.Lookup("subagent"); !ok {
		t.Fatalf("subagent should remain visible for %s", h.ID)
	}
}

type dynamicCapabilityTestRegistry struct{ capabilityTestRegistry }

func (r *dynamicCapabilityTestRegistry) Register(handler tool.Handler) error {
	if r.handlers == nil {
		r.handlers = make(map[string]tool.Handler)
	}
	r.handlers[handler.Definition().Name] = handler
	return nil
}

func (r *dynamicCapabilityTestRegistry) RegisterBatch(handlers []tool.Handler) error {
	for _, handler := range handlers {
		if err := r.Register(handler); err != nil {
			return err
		}
	}
	return nil
}

func (r *dynamicCapabilityTestRegistry) ReplaceNamespace(prefix string, handlers []tool.Handler) error {
	for name := range r.handlers {
		if strings.HasPrefix(name, prefix) {
			delete(r.handlers, name)
		}
	}
	return r.RegisterBatch(handlers)
}

func TestCapabilityRegistryPreservesDynamicRegistrar(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	base := &dynamicCapabilityTestRegistry{capabilityTestRegistry{handlers: map[string]tool.Handler{}}}
	reg := NewCapabilityRegistry(base, coord)
	dynamic, ok := reg.(tool.DynamicRegistrar)
	if !ok {
		t.Fatal("capability registry dropped DynamicRegistrar")
	}
	h := NewSubagent(coord)
	if err := dynamic.Register(h); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Lookup("subagent"); !ok {
		t.Fatal("dynamically registered subagent handler is not visible")
	}
}

type compiledCapabilityTestRegistry struct{ capabilityTestRegistry }

type capabilityDefinitionHandler struct{ definition tool.Definition }

func (h capabilityDefinitionHandler) Definition() tool.Definition { return h.definition }
func (h capabilityDefinitionHandler) Execute(context.Context, tool.Call) (tool.Result, error) {
	return tool.Result{}, nil
}

func (r compiledCapabilityTestRegistry) CompiledValidators(name string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool) {
	handler, ok := r.handlers[name]
	if !ok {
		return nil, nil, false
	}
	definition := handler.Definition()
	input, err := usecase.CompileToolInputValidator(domain.Tool{
		Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema,
	})
	return input, nil, err == nil
}

func TestCapabilityRegistryPreservesCompiledValidatorsForVisibleTools(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	base := compiledCapabilityTestRegistry{capabilityTestRegistry{handlers: map[string]tool.Handler{
		"subagent": NewSubagent(coord),
	}}}
	reg := NewCapabilityRegistry(base, coord)
	compiled, ok := reg.(interface {
		CompiledValidators(string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool)
	})
	if !ok {
		t.Fatal("capability registry dropped compiled validator cache")
	}
	if _, _, found := compiled.CompiledValidators("subagent"); !found {
		t.Fatal("visible subagent did not preserve compiled validators")
	}
	coord.SetEnabled(false)
	if _, _, found := compiled.CompiledValidators("subagent"); found {
		t.Fatal("hidden subagent exposed cached validators")
	}
}

func TestCapabilityRegistryNarrowsSubagentValidatorWhenDisabled(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	if _, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileAgility, Task: "inspect"}); err != nil {
		t.Fatal(err)
	}
	coord.SetEnabled(false)
	originalHandler := NewSubagent(coord)
	base := compiledCapabilityTestRegistry{capabilityTestRegistry{handlers: map[string]tool.Handler{
		tool.NameSubagent: originalHandler,
	}}}
	reg := NewCapabilityRegistry(base, coord)
	definitions := reg.Definitions()
	if len(definitions) != 1 {
		t.Fatalf("definitions = %d, want one lifecycle tool", len(definitions))
	}
	if strings.Contains(definitions[0].Description, "action=spawn") || strings.Contains(definitions[0].Description, "action=resume") {
		t.Fatalf("lifecycle description advertises unavailable actions: %q", definitions[0].Description)
	}
	properties, _ := definitions[0].InputSchema["properties"].(map[string]any)
	for _, removed := range []string{"task", "profile", "context", "taskId", "dependsOn", "optional"} {
		if _, exists := properties[removed]; exists {
			t.Fatalf("lifecycle schema retained spawn-only property %q", removed)
		}
	}
	encodedSchema, err := json.Marshal(definitions[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedSchema), "action=spawn") || strings.Contains(string(encodedSchema), "resume") {
		t.Fatalf("lifecycle schema advertises unavailable actions: %s", encodedSchema)
	}
	compiled, ok := reg.(interface {
		CompiledValidators(string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool)
	})
	if !ok {
		t.Fatal("capability registry dropped compiled validator cache")
	}
	input, _, found := compiled.CompiledValidators(tool.NameSubagent)
	if !found || input == nil {
		t.Fatal("lifecycle-only subagent input validator is unavailable")
	}
	if err := input.Validate([]byte(`{"action":"spawn","task":"more work"}`)); err == nil {
		t.Fatal("lifecycle-only validator accepted action=spawn")
	}
	if err := input.Validate([]byte(`{"action":"cancel","agentId":"agility-1"}`)); err != nil {
		t.Fatalf("lifecycle-only validator rejected action=cancel: %v", err)
	}
	if err := input.Validate([]byte(`{"action":"cancel","agentId":"agility-1","task":"forbidden"}`)); err == nil {
		t.Fatal("lifecycle-only validator accepted a spawn-only property")
	}

	updatedDefinition := originalHandler.Definition()
	updatedDefinition.Description += " updated"
	base.handlers[tool.NameSubagent] = capabilityDefinitionHandler{definition: updatedDefinition}
	updatedInput, _, found := compiled.CompiledValidators(tool.NameSubagent)
	if !found || updatedInput == nil {
		t.Fatal("replacement lifecycle validator is unavailable")
	}
	if updatedInput == input {
		t.Fatal("replacement lifecycle schema reused a stale cached validator")
	}
}
