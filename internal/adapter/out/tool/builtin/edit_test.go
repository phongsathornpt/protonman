package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFacadeDispatchesActions(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	store := &recordingCheckpointStore{id: "cp-edit"}
	handler := NewEdit(ws, store)

	if definition := handler.Definition(); definition.Name != "edit" {
		t.Fatalf("Definition.Name = %q, want edit", definition.Name)
	}

	_, err := handler.Execute(context.Background(), newJSONCall(t, "edit-write", "edit", map[string]any{
		"action": "write", "file_path": "a.txt", "content": "alpha",
	}))
	if err != nil {
		t.Fatalf("write action: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(ws.Root(), "a.txt")); err != nil || string(got) != "alpha" {
		t.Fatalf("write result = %q err=%v", got, err)
	}

	_, err = handler.Execute(context.Background(), newJSONCall(t, "edit-replace", "edit", map[string]any{
		"action": "replace", "file_path": "a.txt", "old_string": "alpha", "new_string": "beta",
	}))
	if err != nil {
		t.Fatalf("replace action: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(ws.Root(), "a.txt")); err != nil || string(got) != "beta" {
		t.Fatalf("replace result = %q err=%v", got, err)
	}

	patch := "*** Begin Patch\n*** Add File: b.txt\n+bravo\n*** End Patch"
	_, err = handler.Execute(context.Background(), newJSONCall(t, "edit-patch", "edit", map[string]any{
		"action": "patch", "patch": patch,
	}))
	if err != nil {
		t.Fatalf("patch action: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(ws.Root(), "b.txt")); err != nil || !strings.Contains(string(got), "bravo") {
		t.Fatalf("patch result = %q err=%v", got, err)
	}

	_, err = handler.Execute(context.Background(), newJSONCall(t, "edit-restore", "edit", map[string]any{
		"action": "restore", "checkpoint_id": "cp-old",
	}))
	if err != nil {
		t.Fatalf("restore action: %v", err)
	}
	if store.restored != "cp-old" {
		t.Fatalf("restored = %q, want cp-old", store.restored)
	}
}

func TestEditFacadeSemanticsFollowAction(t *testing.T) {
	handler := NewEdit(newTestWorkspace(t, nil), &recordingCheckpointStore{id: "cp"})
	definition := handler.Definition()
	tests := []struct {
		action         string
		wantSafety     string
		wantCheckpoint string
	}{
		{"write", "whole_file", "required"},
		{"replace", "contextual", "required"},
		{"patch", "dynamic", "required"},
		{"restore", "whole_file", "none"},
	}
	for _, tt := range tests {
		semantics := definition.Semantics([]byte(`{"action":"` + tt.action + `"}`))
		if string(semantics.Safety.MutationSafety) != tt.wantSafety || string(semantics.Safety.CheckpointPolicy) != tt.wantCheckpoint {
			t.Fatalf("%s semantics = %+v", tt.action, semantics.Safety)
		}
	}
}
