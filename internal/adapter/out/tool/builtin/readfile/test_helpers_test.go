package readfile

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
)

func newTestWorkspace(t *testing.T, protected []string) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.New(t.TempDir(), protected)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	return ws
}

func newJSONCall(t *testing.T, id, name string, input map[string]any) tool.Call {
	t.Helper()
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	call, err := tool.NewCall(id, name, arguments)
	if err != nil {
		t.Fatalf("tool.NewCall() error = %v", err)
	}
	return call
}
