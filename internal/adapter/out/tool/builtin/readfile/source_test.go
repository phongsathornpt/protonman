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

func TestSourceViewSkipsProtectedPaths(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, root, "public/main.go", "needle\n")
	writeSourceTestFile(t, root, "secrets/token.go", "needle secret\n")
	ws, err := workspace.New(root, []string{"secrets"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "source-protected", "read_file", map[string]any{
		"path": ".", "view": "source", "query": "needle",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "public/main.go") || strings.Contains(result.Output, "secrets/token.go") || strings.Contains(result.Output, "needle secret") {
		t.Fatalf("protected source leaked into output: %s", result.Output)
	}
}

func TestSourceViewRejectsEscapingRootSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeSourceTestFile(t, outside, "secret.go", "needle outside\n")
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	ws, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(ws).Execute(context.Background(), newJSONCall(t, "source-symlink", "read_file", map[string]any{
		"path": "outside-link", "view": "source", "query": "needle",
	}))
	if err == nil {
		t.Fatal("source view allowed root symlink outside workspace")
	}
	failure := tool.FailureFromError(err)
	if failure == nil || failure.Code != tool.ErrorCodeOutsideWorkspace {
		t.Fatalf("failure = %#v, want outside_workspace; err=%v", failure, err)
	}
}

func TestSourceViewDoesNotTraverseNestedSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeSourceTestFile(t, root, "inside.go", "needle inside\n")
	writeSourceTestFile(t, outside, "secret.go", "needle outside\n")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	ws, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "source-nested-symlink", "read_file", map[string]any{
		"path": ".", "view": "source", "query": "needle",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "inside.go") || strings.Contains(result.Output, "needle outside") || strings.Contains(result.Output, "linked/") {
		t.Fatalf("nested symlink content leaked: %s", result.Output)
	}
}

func TestReadFileDefinitionPublishesSourceView(t *testing.T) {
	def := readFileHandler{}.Definition()
	properties, ok := def.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", def.InputSchema["properties"])
	}
	view, ok := properties["view"].(map[string]any)
	if !ok {
		t.Fatalf("view schema = %#v", properties["view"])
	}
	values, ok := view["enum"].([]string)
	if !ok {
		t.Fatalf("view enum = %#v", view["enum"])
	}
	found := false
	for _, value := range values {
		if value == "source" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("read_file view enum missing source: %#v", values)
	}
}
