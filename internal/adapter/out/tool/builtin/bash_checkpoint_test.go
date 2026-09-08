package builtin

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestBashCheckpointsProvenRemoveMutation(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "remove.txt", "remove me\n")
	store := &recordingCheckpointStore{id: "bash-rm"}
	result, err := NewBashWithCheckpoint(ws, &recordingLauncher{}, store).Execute(context.Background(),
		newJSONCall(t, "bash-rm", "bash", map[string]any{"command": "rm remove.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.CheckpointID != store.id {
		t.Fatalf("checkpoint id = %q, want %q", result.CheckpointID, store.id)
	}
	if result.MutationCoverage != tool.MutationCoverageFull {
		t.Fatalf("mutation coverage = %q, want full", result.MutationCoverage)
	}
	want := filepath.Join(ws.Root(), "remove.txt")
	if len(store.paths) != 1 || store.paths[0] != want {
		t.Fatalf("checkpoint paths = %#v, want [%s]", store.paths, want)
	}
}

func TestBashCheckpointsMoveSourceAndDestination(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "old.txt", "move me\n")
	store := &recordingCheckpointStore{id: "bash-mv"}
	result, err := NewBashWithCheckpoint(ws, &recordingLauncher{}, store).Execute(context.Background(),
		newJSONCall(t, "bash-mv", "bash", map[string]any{"command": "mv old.txt new.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.CheckpointID != store.id || len(store.paths) != 2 {
		t.Fatalf("result=%#v checkpoint paths=%#v", result, store.paths)
	}
}
func TestBashFailureRetainsCheckpointID(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	store := &recordingCheckpointStore{id: "bash-fail"}
	result, err := NewBashWithCheckpoint(ws, &recordingLauncher{}, store).Execute(context.Background(),
		newJSONCall(t, "bash-fail", "bash", map[string]any{"command": "rm missing.txt"}))
	if err == nil {
		t.Fatal("expected rm failure")
	}
	if result.CheckpointID != store.id {
		t.Fatalf("checkpoint id = %q, want %q", result.CheckpointID, store.id)
	}
}

func TestBashUnknownMutationDoesNotClaimCheckpointCoverage(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	store := &recordingCheckpointStore{id: "bash-unknown"}
	result, err := NewBashWithCheckpoint(ws, &recordingLauncher{}, store).Execute(context.Background(),
		newJSONCall(t, "bash-unknown", "bash", map[string]any{"command": "sh -c 'printf x > unknown.txt'"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.CheckpointID != "" || len(store.paths) != 0 {
		t.Fatalf("unknown mutation claimed checkpoint coverage: id=%q paths=%#v", result.CheckpointID, store.paths)
	}
	if result.MutationCoverage != tool.MutationCoverageUnknown {
		t.Fatalf("mutation coverage = %q, want unknown", result.MutationCoverage)
	}
}
