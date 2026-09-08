package readfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
)

func TestSourceViewCompoundSearch(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, root, "main.go", "package main\n\nfunc run(v string) {\n\tswitch strings.TrimSpace(v) {\n\tcase \"a\":\n\tcase \"b\":\n\t}\n}\n")
	writeSourceTestFile(t, root, "main_test.go", "package main\nfunc TestRun() { switch value { case 1: } }\n")
	writeSourceTestFile(t, root, "pkg/other.go", "package pkg\nfunc other() {\n\tswitch runtime.GOOS {\n\tcase \"linux\":\n\t}\n}\n")
	ws, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "source-1", "read_file", map[string]any{"path": ".", "view": "source", "query": `^\s*switch\b`, "mode": "regex", "include": []string{"**/*.go"}, "exclude": []string{"**/*_test.go"}, "context": map[string]any{"after": 2}}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Output, "main_test.go") || !strings.Contains(result.Output, "main.go:4") || !strings.Contains(result.Output, "pkg/other.go:3") {
		t.Fatalf("unexpected source output: %s", result.Output)
	}
	var structured sourceOutput
	if err := json.Unmarshal(result.StructuredOutput, &structured); err != nil {
		t.Fatal(err)
	}
	if structured.Stats.FilesMatched != 2 || structured.Stats.Matches != 2 {
		t.Fatalf("unexpected structured output: %+v", structured)
	}
}

func TestSourceViewLiteralLimitAndInvalidRegex(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, root, "a.go", "needle\nneedle\n")
	writeSourceTestFile(t, root, "b.go", "needle\n")
	ws, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "source-2", "read_file", map[string]any{"path": ".", "view": "source", "query": "needle", "include": []string{"*.go"}, "max_matches": 1}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Fatal("expected truncated result")
	}
	_, err = New(ws).Execute(context.Background(), newJSONCall(t, "source-3", "read_file", map[string]any{"path": ".", "view": "source", "query": "(", "mode": "regex"}))
	var toolErr *tool.ToolError
	if err == nil || !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeInvalidArguments {
		t.Fatalf("unexpected invalid regex error: %v", err)
	}
}

func writeSourceTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
