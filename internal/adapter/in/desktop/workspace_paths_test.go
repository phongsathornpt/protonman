//go:build desktop

package desktop

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestValidWorkspacePathRequiresExistingAbsoluteDirectory(t *testing.T) {
	dir := t.TempDir()
	if got := validWorkspacePath(dir); got != filepath.Clean(dir) {
		t.Fatalf("valid workspace = %q, want %q", got, filepath.Clean(dir))
	}
	if got := validWorkspacePath("relative/path"); got != "" {
		t.Fatalf("relative workspace = %q, want empty", got)
	}
	if got := validWorkspacePath(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("missing workspace = %q, want empty", got)
	}
}

func TestWorkspacePathMapDropsMissingAndNormalizesKeys(t *testing.T) {
	dir := t.TempDir()
	payload, err := json.Marshal(map[string]string{
		" key ": dir,
		"gone":  filepath.Join(dir, "missing"),
	})
	if err != nil {
		t.Fatal(err)
	}
	paths := workspacePathMap(string(payload))
	if got := paths["key"]; got != filepath.Clean(dir) {
		t.Fatalf("resolved path = %q, want %q", got, filepath.Clean(dir))
	}
	if _, ok := paths["gone"]; ok {
		t.Fatal("missing workspace path was retained")
	}
	if _, ok := paths[" key "]; ok {
		t.Fatal("untrimmed workspace key was retained")
	}
}

func TestWorkspacePathMapFailsClosedOnInvalidJSON(t *testing.T) {
	if got := workspacePathMap("{"); len(got) != 0 {
		t.Fatalf("invalid preferences produced paths: %#v", got)
	}
}
