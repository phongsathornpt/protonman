package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin"
	todotool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/todo"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func TestRegistryForSessionIsolatesTodoState(t *testing.T) {
	baseStore, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err := builtin.NewRegistry(todotool.NewTodo(baseStore))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "sessions")
	runtime := &appRuntime{registry: base, sessionsRoot: root}

	regA, err := runtime.registryForSession("session-a")
	if err != nil {
		t.Fatal(err)
	}
	regB, err := runtime.registryForSession("session-b")
	if err != nil {
		t.Fatal(err)
	}

	updateA, ok := regA.Lookup("todo")
	if !ok {
		t.Fatal("session A todo missing")
	}
	call, err := tool.NewCall("a-update", "todo", json.RawMessage(`{"action":"update","expected_revision":0,"operations":[{"op":"add","id":"a","text":"session A","status":"pending"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updateA.Execute(context.Background(), call); err != nil {
		t.Fatal(err)
	}

	getB, ok := regB.Lookup("todo")
	if !ok {
		t.Fatal("session B todo missing")
	}
	getCall, err := tool.NewCall("b-get", "todo", json.RawMessage(`{"action":"get"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := getB.Execute(context.Background(), getCall)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot tododomain.Snapshot
	if err := json.Unmarshal(result.StructuredOutput, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 0 {
		t.Fatalf("session B leaked session A todo: %+v", snapshot)
	}

	resourcesA, _ := session.ResolveResources(root, "session-a")
	resourcesB, _ := session.ResolveResources(root, "session-b")
	if _, err := os.Stat(resourcesA.Todo); err != nil {
		t.Fatalf("session A todo missing: %v", err)
	}
	if _, err := os.Stat(resourcesB.Todo); !os.IsNotExist(err) {
		t.Fatalf("session B todo unexpectedly created: %v", err)
	}
}
