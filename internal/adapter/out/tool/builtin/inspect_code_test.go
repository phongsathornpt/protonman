package builtin

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

func TestInspectCodeCompoundSearch(t *testing.T) {
	root := t.TempDir()
	mustWriteInspectFile(t, root, "main.go", "package main\n\nfunc run(v string) {\n\tswitch strings.TrimSpace(v) {\n\tcase \"a\":\n\tcase \"b\":\n\t}\n}\n")
	mustWriteInspectFile(t, root, "main_test.go", "package main\nfunc TestRun() { switch value { case 1: } }\n")
	mustWriteInspectFile(t, root, "pkg/other.go", "package pkg\nfunc other() {\n\tswitch runtime.GOOS {\n\tcase \"linux\":\n\t}\n}\n")
	mustWriteInspectFile(t, root, "README.md", "switch docs only\n")

	ws, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewInspectCode(ws)
	call := newJSONCall(t, "inspect-1", "inspect_code", map[string]any{
		"query":   `^\s*switch\b`,
		"mode":    "regex",
		"include": []string{"**/*.go"},
		"exclude": []string{"**/*_test.go"},
		"context": map[string]any{"after": 2},
	})

	result, err := h.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Output, "main_test.go") || strings.Contains(result.Output, "README.md") {
		t.Fatalf("excluded file leaked into output: %s", result.Output)
	}
	if !strings.Contains(result.Output, "main.go:4") || !strings.Contains(result.Output, `case "a"`) {
		t.Fatalf("main.go context missing: %s", result.Output)
	}
	if !strings.Contains(result.Output, "pkg/other.go:3") || !strings.Contains(result.Output, `case "linux"`) {
		t.Fatalf("pkg context missing: %s", result.Output)
	}

	var structured inspectCodeOutput
	if err := json.Unmarshal(result.StructuredOutput, &structured); err != nil {
		t.Fatal(err)
	}
	if structured.Stats.FilesMatched != 2 || structured.Stats.Matches != 2 || len(structured.Matches) != 2 {
		t.Fatalf("unexpected structured output: %+v", structured)
	}
}

func TestInspectCodeLiteralAndLimits(t *testing.T) {
	root := t.TempDir()
	mustWriteInspectFile(t, root, "a.go", "needle\nneedle\n")
	mustWriteInspectFile(t, root, "b.go", "needle\n")
	ws, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewInspectCode(ws).Execute(context.Background(), newJSONCall(t, "inspect-2", "inspect_code", map[string]any{
		"query": "needle", "include": []string{"*.go"}, "max_matches": 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Fatal("expected truncated result")
	}
	var structured inspectCodeOutput
	if err := json.Unmarshal(result.StructuredOutput, &structured); err != nil {
		t.Fatal(err)
	}
	if len(structured.Matches) != 1 || !structured.Truncated {
		t.Fatalf("unexpected result: %+v", structured)
	}
}

func TestInspectCodeRejectsInvalidRegex(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewInspectCode(ws).Execute(context.Background(), newJSONCall(t, "inspect-3", "inspect_code", map[string]any{
		"query": "(", "mode": "regex",
	}))
	if err == nil {
		t.Fatal("expected invalid regex error")
	}
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeInvalidArguments {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustWriteInspectFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
