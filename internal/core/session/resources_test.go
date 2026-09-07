package session

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveResourcesScopesFilesToSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	got, err := ResolveResources(root, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != filepath.Join(root, "session-1") || got.State != filepath.Join(root, "session-1", StateFileName) || got.Todo != filepath.Join(root, "session-1", TodoFileName) {
		t.Fatalf("resources = %+v", got)
	}
}

func TestResolveResourcesRejectsTraversal(t *testing.T) {
	if _, err := ResolveResources(t.TempDir(), "../escape"); err == nil {
		t.Fatal("expected invalid session id")
	}
}

func TestNewIDIsWorkspaceBoundAndUnique(t *testing.T) {
	a := NewID("/workspace/a")
	b := NewID("/workspace/a")
	if a == b {
		t.Fatalf("duplicate session id %q", a)
	}
	if !strings.HasPrefix(a, "workspace-"+WorkspaceKey("/workspace/a")+"-") {
		t.Fatalf("id = %q", a)
	}
}
