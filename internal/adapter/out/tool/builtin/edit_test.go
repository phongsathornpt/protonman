package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestEditFacadeDispatchesActions(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	store := &recordingCheckpointStore{id: "cp-edit"}
	handler := NewEdit(ws, store)

	if definition := handler.Definition(); definition.Name != "edit" {
		t.Fatalf("Definition.Name = %q, want edit", definition.Name)
	}

	_, err := handler.Execute(context.Background(), newJSONCall(t, "edit-write", "edit", map[string]any{
		"action": "write", "filePath": "a.txt", "content": "alpha",
	}))
	if err != nil {
		t.Fatalf("write action: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(ws.Root(), "a.txt")); err != nil || string(got) != "alpha" {
		t.Fatalf("write result = %q err=%v", got, err)
	}

	_, err = handler.Execute(context.Background(), newJSONCall(t, "edit-replace", "edit", map[string]any{
		"action": "replace", "filePath": "a.txt", "oldString": "alpha", "newString": "beta",
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
		"action": "restore", "checkpointId": "cp-old",
	}))
	if err != nil {
		t.Fatalf("restore action: %v", err)
	}
	if store.restored != "cp-old" {
		t.Fatalf("restored = %q, want cp-old", store.restored)
	}
}

func TestEditSchemaRequiresActionSpecificArguments(t *testing.T) {
	definition := NewEdit(nil, nil).Definition()
	validator, err := sdk.CompileToolInputValidator(sdk.Tool{
		Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	valid := []string{
		`{"action":"write","filePath":"a","content":"x"}`,
		`{"action":"replace","filePath":"a","oldString":"x","newString":"y"}`,
		`{"action":"patch","patch":"*** Begin Patch\n*** End Patch"}`,
		`{"action":"restore","checkpointId":"cp"}`,
	}
	for _, raw := range valid {
		if err := validator.Validate(json.RawMessage(raw)); err != nil {
			t.Errorf("valid call rejected %s: %v", raw, err)
		}
	}
	invalid := []string{
		`{"action":"write","filePath":"a"}`,
		`{"action":"replace","filePath":"a","oldString":"x"}`,
		`{"action":"patch"}`,
		`{"action":"restore"}`,
	}
	for _, raw := range invalid {
		if err := validator.Validate(json.RawMessage(raw)); err == nil {
			t.Errorf("invalid call accepted %s", raw)
		}
	}
}

func TestEditAliasesNormalizeBeforeActionDispatch(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewEdit(ws, &recordingCheckpointStore{id: "cp-edit"})
	definition := handler.Definition()
	call := newJSONCall(t, "edit-alias", "edit", map[string]any{"action": "write", "path": "alias.txt", "content": "ok"})
	call.Arguments = tool.NormalizeArgumentsForHandler(handler, definition, call.Arguments)
	validator, err := sdk.CompileToolInputValidator(sdk.Tool{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema})
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(call.Arguments); err != nil {
		t.Fatalf("normalized alias call rejected by schema: %v", err)
	}
	if _, err := handler.Execute(context.Background(), call); err != nil {
		t.Fatalf("write using path alias: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(ws.Root(), "alias.txt")); err != nil || string(got) != "ok" {
		t.Fatalf("alias write = %q, err=%v", got, err)
	}
}

func TestEditProducesDiffStructuredOutput(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	store := &recordingCheckpointStore{id: "cp-edit"}
	handler := NewEdit(ws, store)

	// 1. Write file produces diff with additions
	res, err := handler.Execute(context.Background(), newJSONCall(t, "edit-write", "edit", map[string]any{
		"action": "write", "filePath": "test.go", "content": "package main\n\nfunc Run() {}\n",
	}))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if len(res.StructuredOutput) == 0 {
		t.Fatalf("expected StructuredOutput from write, got empty")
	}
	if !strings.Contains(string(res.StructuredOutput), `"diff":`) || !strings.Contains(string(res.StructuredOutput), `+func Run() {}`) {
		t.Fatalf("expected diff in StructuredOutput: %s", res.StructuredOutput)
	}
	if !strings.Contains(string(res.StructuredOutput), `diff --git a/test.go b/test.go`) {
		t.Fatalf("expected git diff header in StructuredOutput: %s", res.StructuredOutput)
	}
	if !strings.Contains(string(res.StructuredOutput), `new file mode 100644`) {
		t.Fatalf("expected new file mode in StructuredOutput: %s", res.StructuredOutput)
	}

	// 2. Replace produces diff with both additions and deletions
	res, err = handler.Execute(context.Background(), newJSONCall(t, "edit-replace", "edit", map[string]any{
		"action": "replace", "filePath": "test.go", "oldString": "func Run() {}", "newString": "func Run() error {\n\treturn nil\n}",
	}))
	if err != nil {
		t.Fatalf("replace error: %v", err)
	}
	if !strings.Contains(string(res.StructuredOutput), `-func Run() {}`) || !strings.Contains(string(res.StructuredOutput), `+func Run() error {`) {
		t.Fatalf("expected replacement diff in StructuredOutput: %s", res.StructuredOutput)
	}
	if !strings.Contains(string(res.StructuredOutput), `"additions":3`) || !strings.Contains(string(res.StructuredOutput), `"deletions":1`) {
		t.Fatalf("expected additions:3 and deletions:1 in StructuredOutput: %s", res.StructuredOutput)
	}
	if !strings.Contains(string(res.StructuredOutput), `diff --git a/test.go b/test.go`) {
		t.Fatalf("expected git diff header in StructuredOutput: %s", res.StructuredOutput)
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
