package agenttool

import (
	"context"
	"testing"

	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/feature/agent"
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
	h, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileINT, Task: "inspect"})
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
