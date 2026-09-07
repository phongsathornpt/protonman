package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindFilesRecursivelyMatchesGlob(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	for _, path := range []string{"main.go", "internal/a.go", "internal/a_test.go", "docs/readme.md"} {
		full := filepath.Join(ws.Root(), path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := NewFindFiles(ws).Execute(context.Background(), newJSONCall(t, "find-go", "find_files", map[string]any{
		"pattern": "*.go",
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"file main.go", "file internal/a.go", "file internal/a_test.go"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output missing %q:\n%s", want, result.Output)
		}
	}
	if strings.Contains(result.Output, "readme.md") {
		t.Fatalf("output included non-matching file:\n%s", result.Output)
	}
}

func TestFindFilesSupportsDepthTypeAndPagination(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	for _, path := range []string{"top.txt", "one/a.txt", "one/two/b.txt"} {
		full := filepath.Join(ws.Root(), path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewFindFiles(ws)
	result, err := h.Execute(context.Background(), newJSONCall(t, "find-depth", "find_files", map[string]any{
		"pattern": "*", "type": "file", "max_depth": 1, "limit": 1,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Truncated || result.NextOffset != nil || result.Continuation != "" {
		t.Fatalf("unexpected pagination for one depth-1 file: %+v", result)
	}
	if !strings.Contains(result.Output, "top.txt") || strings.Contains(result.Output, "one/a.txt") {
		t.Fatalf("depth filter output = %q", result.Output)
	}

	page1, err := h.Execute(context.Background(), newJSONCall(t, "find-page1", "find_files", map[string]any{
		"pattern": "*.txt", "limit": 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !page1.Truncated || page1.NextOffset == nil || page1.Continuation == "" {
		t.Fatalf("page1 pagination = %+v", page1)
	}
	page2, err := h.Execute(context.Background(), newJSONCall(t, "find-page2", "find_files", map[string]any{
		"pattern": "*.txt", "limit": 1, "offset": *page1.NextOffset, "continuation": page1.Continuation,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if page2.Output == page1.Output {
		t.Fatalf("page2 repeated page1: %q", page2.Output)
	}
}

func TestFindFilesSkipsProtectedPaths(t *testing.T) {
	ws := newTestWorkspace(t, []string{"secret/**"})
	if err := os.MkdirAll(filepath.Join(ws.Root(), "secret"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Root(), "secret", "token.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Root(), "visible.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewFindFiles(ws).Execute(context.Background(), newJSONCall(t, "find-protected", "find_files", map[string]any{"pattern": "*.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Output, "token.txt") || !strings.Contains(result.Output, "visible.txt") {
		t.Fatalf("protected filtering output = %q", result.Output)
	}
}

func TestFindFilesSkipsGeneratedAndVCSDirectories(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	for _, path := range []string{"visible.go", ".git/hidden.go", "node_modules/pkg/hidden.go", "target/debug/hidden.go"} {
		full := filepath.Join(ws.Root(), path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := NewFindFiles(ws).Execute(context.Background(), newJSONCall(t, "find-ignore", "find_files", map[string]any{"pattern": "*.go"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "visible.go") {
		t.Fatalf("output missing visible file: %q", result.Output)
	}
	for _, hidden := range []string{".git/hidden.go", "node_modules/pkg/hidden.go", "target/debug/hidden.go"} {
		if strings.Contains(result.Output, hidden) {
			t.Fatalf("output included ignored path %q: %q", hidden, result.Output)
		}
	}
}

func TestFindFilesContinuationSurvivesChangedTree(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(ws.Root(), name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewFindFiles(ws)
	page1, err := h.Execute(context.Background(), newJSONCall(t, "find-stale-1", "find_files", map[string]any{"pattern": "*.txt", "limit": 1}))
	if err != nil {
		t.Fatal(err)
	}
	if page1.NextOffset == nil || page1.Continuation == "" {
		t.Fatalf("page1 missing continuation: %+v", page1)
	}
	if err := os.WriteFile(filepath.Join(ws.Root(), "0.txt"), []byte("new prefix"), 0o644); err != nil {
		t.Fatal(err)
	}
	page2, err := h.Execute(context.Background(), newJSONCall(t, "find-stale-2", "find_files", map[string]any{
		"pattern": "*.txt", "limit": 1, "offset": *page1.NextOffset, "continuation": page1.Continuation,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v, want best-effort continuation", err)
	}
	if strings.TrimSpace(page2.Output) == "" {
		t.Fatalf("page2 output = %q", page2.Output)
	}
}
