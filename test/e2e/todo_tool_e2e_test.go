package e2e_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin"
	todotool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/todo"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
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
	registry, err := builtin.NewRegistry(todotool.NewTodo(store))
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

	registry2, err := builtin.NewRegistry(todotool.NewTodo(restarted))
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
	if final.Revision != 2 || final.Items[1].Status != tododomain.StatusCompleted || final.Items[2].Status != tododomain.StatusInProgress {
		t.Fatalf("final snapshot = %#v", final)
	}
}

func TestE2ETodoLivesInSessionAggregateNotWorkspace(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	sessionID := "todo-session-scope"
	args := `{"action":"update","expected_revision":0,"operations":[{"op":"add","id":"inspect","text":"Inspect session todo","status":"in_progress"}]}`
	res := runProton(t, runOptions{
		args: []string{"-y", "-s", sessionID, "-p", "/call todo " + args},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("todo update failed: %s\n%s", res.stdout, res.stderr)
	}
	todoPath := filepath.Join(home, ".protonman", "sessions", sessionID, "todo.md")
	contents, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatalf("session todo missing at %s: %v", todoPath, err)
	}
	if !strings.Contains(string(contents), "[inspect] Inspect session todo") {
		t.Fatalf("session todo content missing: %s", contents)
	}
	if _, err := os.Stat(filepath.Join(ws, "TODO.md")); !os.IsNotExist(err) {
		t.Fatalf("workspace TODO.md should remain unmanaged, stat err=%v", err)
	}
}
