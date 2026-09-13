package memory

import "testing"

func TestEntryValidateWorkspaceScope(t *testing.T) {
	entry := Entry{
		ID: "mem-1", Scope: ScopeWorkspace, Kind: KindRepoFact,
		Key: "test-command", Value: "go test ./...", WorkspaceKey: "workspace-1", Confidence: 0.9,
	}
	if err := entry.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEntryValidateRejectsWorkspaceScopeWithoutWorkspaceKey(t *testing.T) {
	entry := Entry{ID: "mem-1", Scope: ScopeWorkspace, Kind: KindRepoFact, Key: "k", Value: "v", Confidence: 1}
	if err := entry.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want workspace binding error")
	}
}

func TestEntryValidateRejectsGlobalWorkspaceBinding(t *testing.T) {
	entry := Entry{ID: "mem-1", Scope: ScopeGlobal, Kind: KindPreference, Key: "k", Value: "v", WorkspaceKey: "workspace-1", Confidence: 1}
	if err := entry.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want global workspace binding error")
	}
}

func TestEntryValidateRejectsInvalidConfidence(t *testing.T) {
	entry := Entry{ID: "mem-1", Scope: ScopeGlobal, Kind: KindPreference, Key: "k", Value: "v", Confidence: 1.1}
	if err := entry.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want confidence error")
	}
}
