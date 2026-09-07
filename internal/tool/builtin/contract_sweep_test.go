package builtin

import (
	"encoding/json"
	"testing"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/tool"
)

func TestRegisteredBuiltinToolContracts(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()

	primary, err := NewDefaultRegistry(workspaceRoot, testSandboxOption(), testCheckpointOption(), WithAgentCoordinator(coord))
	if err != nil {
		t.Fatal(err)
	}
	auxiliary, err := NewRegistry(NewGetTodo(nil), NewUpdateTodo(nil), NewActivateSkill(nil, workspaceRoot))
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{
		"read_file": true, "bash": true, "write_file": true, "search_replace": true,
		"apply_patch": true, "grep": true, "list_dir": true, "git_status": true,
		"checkpoint_restore": true, "web_fetch": true, "delegate_task": true,
		"wait_agent": true, "get_agent": true, "list_agents": true, "cancel_agent": true,
		"get_todo": true, "update_todo": true, "activate_skill": true,
	}
	seen := make(map[string]bool, len(expected))
	for _, registry := range []*Registry{primary, auxiliary} {
		for _, definition := range registry.Definitions() {
			seen[definition.Name] = true
			assertBuiltinContract(t, registry, definition)
		}
	}
	if len(seen) != len(expected) {
		t.Fatalf("registered built-ins = %d, want %d: %#v", len(seen), len(expected), seen)
	}
	for name := range expected {
		if !seen[name] {
			t.Errorf("built-in %q is missing from contract sweep", name)
		}
	}
}
func assertBuiltinContract(t *testing.T, registry *Registry, definition tool.Definition) {
	t.Helper()
	if err := definition.Validate(); err != nil {
		t.Errorf("%s Definition.Validate() error = %v", definition.Name, err)
	}
	if err := definition.Safety.Validate(); err != nil {
		t.Errorf("%s Safety.Validate() error = %v; contract=%+v", definition.Name, err, definition.Safety)
	}
	metadata, ok := tool.MetadataForName(definition.Name)
	if !ok {
		t.Errorf("%s canonical metadata missing", definition.Name)
	} else {
		if metadata.Kind != definition.Kind {
			t.Errorf("%s metadata kind = %q, definition kind = %q", definition.Name, metadata.Kind, definition.Kind)
		}
		if metadata.DisplayName == "" {
			t.Errorf("%s metadata display name is empty", definition.Name)
		}
	}
	if tool.EffectiveMutability(definition) == tool.MutabilityReadOnly && definition.Safety.MutationDomain != tool.MutationDomainNone {
		t.Errorf("%s read-only tool declares mutation domain %q", definition.Name, definition.Safety.MutationDomain)
	}
	input, output, ok := registry.CompiledValidators(definition.Name)
	if !ok {
		t.Errorf("%s has no cached compiled validators", definition.Name)
		return
	}
	if len(definition.InputSchema) > 0 {
		if input == nil {
			t.Errorf("%s input validator = nil", definition.Name)
		}
		if got := definition.InputSchema["type"]; got != "object" {
			t.Errorf("%s input schema type = %#v, want object", definition.Name, got)
		}
		if got := definition.InputSchema["additionalProperties"]; got != false {
			t.Errorf("%s additionalProperties = %#v, want false", definition.Name, got)
		}
	}
	if len(definition.OutputSchema) > 0 && output == nil {
		t.Errorf("%s output validator = nil", definition.Name)
	}
}
func TestOptionalZeroNumericArgumentsMatchOmittedSemantics(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	registry, err := NewDefaultRegistry(workspaceRoot, testSandboxOption(), testCheckpointOption(), WithAgentCoordinator(coord))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		zeroArgs map[string]any
		badArgs  map[string]any
	}{
		{"read_file", map[string]any{"path": "x", "limit": 0}, map[string]any{"path": "x", "limit": -1}},
		{"list_dir", map[string]any{"limit": 0}, map[string]any{"limit": -1}},
		{"grep", map[string]any{"pattern": "x", "limit": 0}, map[string]any{"pattern": "x", "limit": -1}},
		{"bash", map[string]any{"command": "true", "timeout_seconds": 0}, map[string]any{"command": "true", "timeout_seconds": -1}},
		{"delegate_task", map[string]any{"task": "inspect", "profile": "int", "timeout_seconds": 0}, map[string]any{"task": "inspect", "profile": "int", "timeout_seconds": -1}},
		{"wait_agent", map[string]any{"agent_id": "agent-1", "timeout_seconds": 0}, map[string]any{"agent_id": "agent-1", "timeout_seconds": -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, _, ok := registry.CompiledValidators(tc.name)
			if !ok || input == nil {
				t.Fatalf("compiled input validator missing for %s", tc.name)
			}
			zeroPayload, err := json.Marshal(tc.zeroArgs)
			if err != nil {
				t.Fatal(err)
			}
			if err := input.Validate(zeroPayload); err != nil {
				t.Fatalf("explicit zero rejected: %v", err)
			}
			badPayload, err := json.Marshal(tc.badArgs)
			if err != nil {
				t.Fatal(err)
			}
			if err := input.Validate(badPayload); err == nil {
				t.Fatal("negative optional numeric value unexpectedly accepted")
			}
		})
	}
}
