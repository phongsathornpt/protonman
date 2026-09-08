package agenttool

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type capabilityTestRegistry struct{ handlers map[string]tool.Handler }

func (r capabilityTestRegistry) Lookup(name string) (tool.Handler, bool) {
	h, ok := r.handlers[name]
	return h, ok
}
func (r capabilityTestRegistry) Definitions() []tool.Definition {
	defs := make([]tool.Definition, 0, len(r.handlers))
	for _, name := range []string{"delegate_task", "wait_agent", "get_agent", "list_agents", "cancel_agent"} {
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
		"delegate_task": NewDelegateTask(coord), "wait_agent": NewWaitAgent(coord), "get_agent": NewGetAgent(coord),
		"list_agents": NewListAgents(coord), "cancel_agent": NewCancelAgent(coord),
	}}
	reg := NewCapabilityRegistry(base, coord)
	if got := reg.Definitions(); len(got) != 0 {
		t.Fatalf("definitions = %d, want 0", len(got))
	}
	if _, ok := reg.Lookup("delegate_task"); ok {
		t.Fatal("delegate_task should be hidden")
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
		"delegate_task": NewDelegateTask(coord), "wait_agent": NewWaitAgent(coord), "get_agent": NewGetAgent(coord),
		"list_agents": NewListAgents(coord), "cancel_agent": NewCancelAgent(coord),
	}}
	reg := NewCapabilityRegistry(base, coord)
	if _, ok := reg.Lookup("delegate_task"); ok {
		t.Fatal("delegate_task should be hidden after disable")
	}
	for _, name := range []string{"wait_agent", "get_agent", "list_agents", "cancel_agent"} {
		if _, ok := reg.Lookup(name); !ok {
			t.Fatalf("%s should remain visible for %s", name, h.ID)
		}
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
	h := NewListAgents(coord)
	if err := dynamic.Register(h); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Lookup("list_agents"); !ok {
		t.Fatal("dynamically registered handler is not visible")
	}
}

type compiledCapabilityTestRegistry struct{ capabilityTestRegistry }

func (r compiledCapabilityTestRegistry) CompiledValidators(name string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool) {
	_, ok := r.handlers[name]
	return nil, nil, ok
}

func TestCapabilityRegistryPreservesCompiledValidatorsForVisibleTools(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	base := compiledCapabilityTestRegistry{capabilityTestRegistry{handlers: map[string]tool.Handler{
		"list_agents": NewListAgents(coord),
	}}}
	reg := NewCapabilityRegistry(base, coord)
	compiled, ok := reg.(interface {
		CompiledValidators(string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool)
	})
	if !ok {
		t.Fatal("capability registry dropped compiled validator cache")
	}
	if _, _, found := compiled.CompiledValidators("list_agents"); !found {
		t.Fatal("visible tool did not preserve compiled validators")
	}
	coord.SetEnabled(false)
	if _, _, found := compiled.CompiledValidators("list_agents"); found {
		t.Fatal("hidden tool exposed cached validators")
	}
}
