package e2e_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/permission"
	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/tool/builtin"
	"github.com/projectTHORN/proton/internal/toolcall"
)

func TestE2ETodoToolPersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "TODO.md")
	prefix := "# Release plan\n\nHuman notes stay here.\n"
	if err := os.WriteFile(path, []byte(prefix), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := tododomain.OpenMarkdownStore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := builtin.NewRegistry(builtin.NewUpdateTodo(store))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(permission.ModeAlwaysApprove))
	if err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]any{"expected_revision": uint64(0), "operations": []map[string]any{
		{"op": "add", "id": "inspect", "text": "Inspect router", "status": "completed"},
		{"op": "add", "id": "fix", "text": "Fix cache invalidation", "status": "in_progress"},
		{"op": "add", "id": "test", "text": "Add integration tests", "status": "pending"},
	}})
	call, err := tool.NewCall("todo-1", "update_todo", args)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Call(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	var updatePayload map[string]any
	if err := json.Unmarshal(result.StructuredOutput, &updatePayload); err != nil {
		t.Fatal(err)
	}
	if updatePayload["completed"] != float64(1) || updatePayload["in_progress"] != float64(1) {
		t.Fatalf("structured result = %#v", updatePayload)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(contents), prefix) {
		t.Fatalf("human markdown was changed:\n%s", contents)
	}
	if !strings.Contains(string(contents), "<!-- proton:todos:start -->") || !strings.Contains(string(contents), "- [~] [fix] Fix cache invalidation") {
		t.Fatalf("managed todo section missing:\n%s", contents)
	}

	restarted, err := tododomain.OpenMarkdownStore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := restarted.Snapshot()
	if len(snapshot.Items) != 3 || snapshot.Items[1].ID != "fix" || snapshot.Items[1].Status != tododomain.StatusInProgress {
		t.Fatalf("restart snapshot = %#v", snapshot)
	}

	registry2, err := builtin.NewRegistry(builtin.NewUpdateTodo(restarted))
	if err != nil {
		t.Fatal(err)
	}
	service2, err := toolcall.NewService(registry2, policy, toolcall.WithMode(permission.ModeAlwaysApprove))
	if err != nil {
		t.Fatal(err)
	}
	args2, _ := json.Marshal(map[string]any{"expected_revision": restarted.Snapshot().Revision, "operations": []map[string]any{
		{"op": "set_status", "id": "fix", "status": "completed"},
		{"op": "set_status", "id": "test", "status": "in_progress"},
	}})
	call2, _ := tool.NewCall("todo-2", "update_todo", args2)
	if _, err := service2.Call(ctx, call2); err != nil {
		t.Fatal(err)
	}
	final := restarted.Snapshot()
	if final.Revision != 1 || final.Items[1].Status != tododomain.StatusCompleted || final.Items[2].Status != tododomain.StatusInProgress {
		t.Fatalf("final snapshot = %#v", final)
	}
}
