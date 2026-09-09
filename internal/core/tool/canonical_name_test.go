package tool

import (
	"encoding/json"
	"testing"
)

func TestLegacyToolAliasesNormalizeToCanonicalCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		action string
	}{
		{"web_fetch", "web", "fetch"},
		{"web_search", "web", "search"},
		{"git_status", "git", "status"},
		{"write_file", "edit", "write"},
		{"search_replace", "edit", "replace"},
		{"apply_patch", "edit", "patch"},
		{"checkpoint_restore", "edit", "restore"},
		{"get_todo", "todo", "get"},
		{"update_todo", "todo", "update"},
		{"delegate_task", "subagent", "spawn"},
		{"wait_agent", "subagent", "wait"},
		{"get_agent", "subagent", "get"},
		{"list_agents", "subagent", "list"},
		{"cancel_agent", "subagent", "cancel"},
		{"resume_agent", "subagent", "resume"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := NormalizeLegacyCall(Call{Name: tt.name, Arguments: json.RawMessage(`{}`)})
			if call.Name != tt.want {
				t.Fatalf("name = %q, want %q", call.Name, tt.want)
			}
			if !IsLegacyName(tt.name) {
				t.Fatalf("%q not recognized as legacy", tt.name)
			}
			if tt.action != "" && ExtractString(call.ArgumentsMap(), "action") != tt.action {
				t.Fatalf("action = %q, want %q", ExtractString(call.ArgumentsMap(), "action"), tt.action)
			}
			if again := NormalizeLegacyCall(call); again.Name != call.Name || string(again.Arguments) != string(call.Arguments) {
				t.Fatalf("normalization is not idempotent: first=%#v second=%#v", call, again)
			}
		})
	}
}
