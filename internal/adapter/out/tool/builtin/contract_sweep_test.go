package builtin

import (
	"encoding/json"
	"strings"
	"testing"

	skilltool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/skill"
	todotool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/todo"
	webtool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/web"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
)

func TestRegisteredBuiltinToolContracts(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()

	primary, err := NewDefaultRegistry(workspaceRoot, testSandboxOption(), testCheckpointOption(), WithAdditionalHandlers(webtool.NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkBlocked})), withAgentTools(coord))
	if err != nil {
		t.Fatal(err)
	}
	auxiliary, err := NewRegistry(todotool.NewTodo(nil), skilltool.NewActivateSkill(nil, workspaceRoot))
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{
		"read": true, "math": true, "bash": true, "edit": true,
		"grep": true, "find": true, "ls": true, "git": true,
		"web": true, "subagent": true,
		"todo": true, "skill": true,
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
	assertEnumsSurviveNormalization(t, definition)
	assertIntegerFieldsSurviveNormalization(t, definition)
}

// assertIntegerFieldsSurviveNormalization guards the invariant that an integer
// input field accepts every JSON spelling its schema accepts. JSON Schema treats
// 3.0 and 1e2 as integers, so the validator admits them, but encoding/json cannot
// decode either into a Go int; without canonicalization the call would pass
// validation and then fail inside the handler with no schema diagnostic.
func assertIntegerFieldsSurviveNormalization(t *testing.T, definition tool.Definition) {
	t.Helper()
	properties, ok := definition.InputSchema["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, raw := range properties {
		field, ok := raw.(map[string]any)
		if !ok || field["type"] != "integer" {
			continue
		}
		for _, spelling := range []string{"3.0", "1e2", "100.00"} {
			payload, err := json.Marshal(map[string]any{name: json.RawMessage(spelling)})
			if err != nil {
				t.Fatalf("%s encode probe: %v", definition.Name, err)
			}
			normalized := tool.NormalizeArguments(definition, payload)
			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(normalized, &decoded); err != nil {
				t.Errorf("%s normalized %s probe is not JSON: %v", definition.Name, name, err)
				continue
			}
			value := strings.TrimSpace(string(decoded[name]))
			if strings.ContainsAny(value, ".eE") {
				t.Errorf("%s field %s: integral spelling %s normalized to %s, want integer form",
					definition.Name, name, spelling, value)
			}
		}
	}
}

// assertEnumsSurviveNormalization guards the invariant that a published input
// enum is never stricter than the normalization pipeline. A case-variant of an
// enum value must normalize to the canonical entry, otherwise the schema
// rejects a call the handler's case-insensitive dispatch would have accepted.
func assertEnumsSurviveNormalization(t *testing.T, definition tool.Definition) {
	t.Helper()
	properties, ok := definition.InputSchema["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, raw := range properties {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if typeName, _ := field["type"].(string); typeName != "" && typeName != "string" {
			continue
		}
		values := enumValueStrings(field["enum"])
		if len(values) == 0 {
			continue
		}
		for _, value := range values {
			payload, err := json.Marshal(map[string]any{name: strings.ToUpper(value)})
			if err != nil {
				t.Fatalf("%s encode probe: %v", definition.Name, err)
			}
			normalized := tool.NormalizeArguments(definition, payload)
			var decoded map[string]any
			if err := json.Unmarshal(normalized, &decoded); err != nil {
				t.Errorf("%s normalized %s probe is not JSON: %v", definition.Name, name, err)
				continue
			}
			if decoded[name] != value {
				t.Errorf("%s field %s: uppercase %q normalized to %#v, want %q",
					definition.Name, name, value, decoded[name], value)
			}
		}
	}
}

// enumValueStrings returns the string entries of an enum that survived the
// registry's JSON round-trip, which stores them as []any.
func enumValueStrings(raw any) []string {
	var entries []any
	switch values := raw.(type) {
	case []any:
		entries = values
	case []string:
		entries = make([]any, 0, len(values))
		for _, value := range values {
			entries = append(entries, value)
		}
	default:
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if text, ok := entry.(string); ok {
			out = append(out, text)
		}
	}
	return out
}
func TestOptionalZeroNumericArgumentsMatchOmittedSemantics(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	registry, err := NewDefaultRegistry(workspaceRoot, testSandboxOption(), testCheckpointOption(), WithAdditionalHandlers(webtool.NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkBlocked})), withAgentTools(coord))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		zeroArgs map[string]any
		badArgs  map[string]any
	}{
		{"read", map[string]any{"path": "x", "limit": 0}, map[string]any{"path": "x", "limit": -1}},
		{"ls", map[string]any{"limit": 0}, map[string]any{"limit": -1}},
		{"grep", map[string]any{"pattern": "x", "limit": 0}, map[string]any{"pattern": "x", "limit": -1}},
		{"bash", map[string]any{"command": "true", "timeout_seconds": 0}, map[string]any{"command": "true", "timeout_seconds": -1}},
		{"subagent_spawn", map[string]any{"action": "spawn", "task": "inspect", "profile": "agility", "timeout_seconds": 0}, map[string]any{"action": "spawn", "task": "inspect", "profile": "agility", "timeout_seconds": -1}},
		{"subagent_wait", map[string]any{"action": "wait", "timeout_seconds": 0}, map[string]any{"action": "wait", "timeout_seconds": -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookupName := tc.name
			if strings.HasPrefix(lookupName, "subagent_") {
				lookupName = "subagent"
			}
			input, _, ok := registry.CompiledValidators(lookupName)
			if !ok || input == nil {
				t.Fatalf("compiled input validator missing for %s", lookupName)
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
