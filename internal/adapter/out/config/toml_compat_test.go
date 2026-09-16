package config

import (
	"strings"
	"testing"
)

func TestDecodeTOMLCompat(t *testing.T) {
	raw := `
# Global comment
[permission]
default = "ask"

[[permission.rules]]
tool = "bash"
action = "allow"
pattern = "git status"

[[permission.rules]]
tool = "read"
action = "deny"
pattern = ".env"

[workspace]
protected_paths = [".git", ".env", "secrets/"]

[agent]
subagents_enabled = true
max_tool_calls = 25
profile = "universal"
reasoning_effort = "auto"

[agent.subagents.strength]
provider = "protonman"
model = "coding-model"
reasoning_effort = "high"

[model]
default = "gpt-4o"
provider = "openai"

[skills]
active = ["golang-code-style", "golang-performance"]
`

	var doc fileDocument
	if err := decodeTOML([]byte(raw), &doc); err != nil {
		t.Fatalf("decodeTOML() error = %v", err)
	}

	if doc.Permission.Default != "ask" {
		t.Errorf("Permission.Default = %q, want 'ask'", doc.Permission.Default)
	}
	if len(doc.Permission.Rules) != 2 {
		t.Fatalf("Permission.Rules count = %d, want 2", len(doc.Permission.Rules))
	}
	if doc.Permission.Rules[0].Tool != "bash" || doc.Permission.Rules[0].Action != "allow" {
		t.Errorf("Rule[0] = %+v", doc.Permission.Rules[0])
	}
	if doc.Permission.Rules[1].Tool != "read" || doc.Permission.Rules[1].Action != "deny" {
		t.Errorf("Rule[1] = %+v", doc.Permission.Rules[1])
	}

	if len(doc.Workspace.ProtectedPaths) != 3 {
		t.Errorf("Workspace.ProtectedPaths = %v", doc.Workspace.ProtectedPaths)
	}

	if doc.Agent.SubagentsEnabled == nil || !*doc.Agent.SubagentsEnabled {
		t.Errorf("Agent.SubagentsEnabled = %v, want true", doc.Agent.SubagentsEnabled)
	}
	if doc.Agent.MaxToolCalls == nil || *doc.Agent.MaxToolCalls != 25 {
		t.Errorf("Agent.MaxToolCalls = %v, want 25", doc.Agent.MaxToolCalls)
	}
	if doc.Agent.Profile == nil || *doc.Agent.Profile != "universal" {
		t.Errorf("Agent.Profile = %v, want 'universal'", doc.Agent.Profile)
	}

	sub, ok := doc.Agent.Subagents["strength"]
	if !ok {
		t.Fatalf("Agent.Subagents missing 'strength'")
	}
	if sub.Provider != "protonman" || sub.Model != "coding-model" {
		t.Errorf("subagent strength = %+v", sub)
	}

	if doc.Model.Default != "gpt-4o" || doc.Model.Provider != "openai" {
		t.Errorf("Model = %+v", doc.Model)
	}

	if doc.Skills == nil || len(doc.Skills.Active) != 2 {
		t.Errorf("Skills.Active = %v", doc.Skills)
	}
}

func TestDecodeTOMLCompatSyntaxErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"missing equals", "[model]\nfoo bar"},
		{"unclosed array", "[workspace]\nprotected_paths = [\"a\", \"b\""},
		{"empty key", "[model]\n= 123"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var doc fileDocument
			err := decodeTOML([]byte(tc.raw), &doc)
			if err == nil {
				t.Fatalf("expected syntax error for %q, got nil", tc.raw)
			}
			if !strings.Contains(err.Error(), "line") && !strings.Contains(err.Error(), "syntax") {
				t.Errorf("error %q should indicate line or syntax", err.Error())
			}
		})
	}
}
