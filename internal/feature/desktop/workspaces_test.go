package desktop

import "testing"

func TestGroupSessionsByWorkspaceUsesDurableKey(t *testing.T) {
	sessions := []SessionState{
		{ID: "s1", WorkspaceKey: "abc", WorkspaceName: "protonman", Workspace: "/tmp/old"},
		{ID: "s2", WorkspaceKey: "abc", WorkspaceName: "protonman", Workspace: "/tmp/new"},
		{ID: "s3", WorkspaceKey: "def", WorkspaceName: "ts-pro", Workspace: "/tmp/ts-pro"},
	}
	groups := GroupSessionsByWorkspace(sessions)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].Key != "key:abc" || groups[0].Name != "protonman" || len(groups[0].Sessions) != 2 {
		t.Fatalf("first group = %#v", groups[0])
	}
	if groups[1].Key != "key:def" || groups[1].Name != "ts-pro" || len(groups[1].Sessions) != 1 {
		t.Fatalf("second group = %#v", groups[1])
	}
}

func TestGroupSessionsByWorkspaceFallsBackToPathThenName(t *testing.T) {
	sessions := []SessionState{
		{ID: "path", Workspace: "/work/protonman"},
		{ID: "name-1", WorkspaceName: "legacy"},
		{ID: "name-2", WorkspaceName: "LEGACY"},
	}
	groups := GroupSessionsByWorkspace(sessions)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].Name != "protonman" || groups[0].Key != "path:/work/protonman" {
		t.Fatalf("path group = %#v", groups[0])
	}
	if groups[1].Name != "legacy" || len(groups[1].Sessions) != 2 {
		t.Fatalf("name group = %#v", groups[1])
	}
}
