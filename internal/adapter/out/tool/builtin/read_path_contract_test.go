package builtin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin/readfile"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestReadOnlyToolsClassifyMissingTargets(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	tests := []struct {
		name    string
		handler tool.Handler
		args    map[string]any
	}{
		{name: "read_file source", handler: readfile.New(ws), args: map[string]any{"path": "missing/src", "view": "source", "query": "main"}},
		{name: "list_dir", handler: NewListDir(ws), args: map[string]any{"path": "missing/src"}},
		{name: "find_files", handler: NewFindFiles(ws), args: map[string]any{"path": "missing/src", "pattern": "*.go"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			call, err := tool.NewCall("missing-"+tt.name, tt.name, raw)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tt.handler.Execute(context.Background(), call)
			if err == nil {
				t.Fatal("Execute() error = nil, want missing target failure")
			}
			failure := tool.FailureFromError(err)
			if failure == nil || failure.Code != tool.ErrorCodeNotFound {
				t.Fatalf("failure = %#v, want not_found", failure)
			}
		})
	}
}
