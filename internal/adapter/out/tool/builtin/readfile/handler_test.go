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
)

func TestReadSchemaKeepsArtifactContractCompact(t *testing.T) {
	definition := readFileHandler{}.Definition()
	properties, ok := definition.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", definition.InputSchema["properties"])
	}
	for _, removed := range []string{"query", "mode", "include", "exclude", "context", "max_files", "max_matches"} {
		if _, exists := properties[removed]; exists {
			t.Fatalf("read schema leaked search field %q", removed)
		}
	}
	view, _ := properties["view"].(map[string]any)
	if view["default"] != "auto" {
		t.Fatalf("view default = %#v, want auto", view["default"])
	}
	for _, field := range []string{"offset", "limit", "continuation", "start_line", "end_line", "line_numbers"} {
		schema, _ := properties[field].(map[string]any)
		description, _ := schema["description"].(string)
		if !strings.Contains(strings.ToLower(description), "text-only") {
			t.Fatalf("%s description = %q, want text-only semantics", field, description)
		}
	}
}

func TestReadFileSupportsLineRangesAndNumbers(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	path := filepath.Join(ws.Root(), "lines.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\ndelta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-lines", "read", map[string]any{
		"path": "lines.txt", "start_line": 2, "end_line": 3, "line_numbers": true,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "     2\tbeta\n     3\tgamma\n" {
		t.Fatalf("Output = %q", result.Output)
	}
	if result.SHA256 != "" || result.NextOffset != nil || result.Continuation != "" {
		t.Fatalf("line read unexpectedly published byte-pagination metadata: %+v", result)
	}
}

func TestReadFileLineRangeCanReadFromStartThroughEndLine(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.WriteFile(filepath.Join(ws.Root(), "lines.txt"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-lines-end", "read", map[string]any{
		"path": "lines.txt", "end_line": 2,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "a\nb\n" {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestReadFileRejectsMixedByteAndLinePagination(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.WriteFile(filepath.Join(ws.Root(), "lines.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-lines-bad", "read", map[string]any{
		"path": "lines.txt", "offset": 1, "start_line": 2,
	}))
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("Execute() error = %v, want mixed pagination rejection", err)
	}
}

func TestReadFileRejectsTextPaginationForArtifactViews(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.WriteFile(filepath.Join(ws.Root(), "data.json"), []byte(`{"value":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{name: "structured-limit", args: map[string]any{"path": "data.json", "view": "structured", "limit": 128}},
		{name: "metadata-offset", args: map[string]any{"path": "data.json", "view": "metadata", "offset": 1}},
		{name: "metadata-continuation", args: map[string]any{"path": "data.json", "view": "metadata", "continuation": "stale"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(ws).Execute(context.Background(), newJSONCall(t, tc.name, "read", tc.args))
			if err == nil || !strings.Contains(err.Error(), "do not accept text pagination") {
				t.Fatalf("Execute() error = %v, want artifact pagination rejection", err)
			}
		})
	}
}

func TestReadFileLineRangeHonorsOutputLimit(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.WriteFile(filepath.Join(ws.Root(), "lines.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-lines-limit", "read", map[string]any{
		"path": "lines.txt", "start_line": 1, "end_line": 3, "limit": 7,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || !strings.Contains(result.Output, "alpha\n") || !strings.Contains(result.Output, "output truncated") {
		t.Fatalf("limited line read = %+v", result)
	}
	if result.Pagination == nil || result.Pagination.Kind != "line" || result.Pagination.NextLine == nil || *result.Pagination.NextLine != 2 {
		t.Fatalf("line pagination = %+v", result.Pagination)
	}
}

func TestReadFileRejectsBinaryNULBeyondDetectionHeader(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	data := append([]byte(strings.Repeat("a", 600)), 0, 'b')
	if err := os.WriteFile(filepath.Join(ws.Root(), "binary-ish.dat"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-binary-nul", "read", map[string]any{
		"path": "binary-ish.dat",
	}))
	if err == nil || !strings.Contains(err.Error(), "contains NUL bytes") {
		t.Fatalf("Execute() error = %v, want binary NUL rejection", err)
	}
}

func TestReadFileLineRangeRejectsBinaryNUL(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	data := append([]byte("a\nb"), byte(0))
	data = append(data, []byte("c\n")...)
	if err := os.WriteFile(filepath.Join(ws.Root(), "binary-lines.dat"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-lines-nul", "read", map[string]any{
		"path": "binary-lines.dat", "start_line": 2, "end_line": 2,
	}))
	if err == nil || !strings.Contains(err.Error(), "contains NUL bytes") {
		t.Fatalf("Execute() error = %v, want binary NUL rejection", err)
	}
}

func TestReadFileLineRangeRejectsInvalidUTF8(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.WriteFile(filepath.Join(ws.Root(), "invalid.txt"), []byte{'a', '\n', 0xff, '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-lines-utf8", "read", map[string]any{
		"path": "invalid.txt", "start_line": 2, "end_line": 2,
	}))
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("Execute() error = %v, want UTF-8 rejection", err)
	}
}

func TestReadFileLineRangeBoundsScannedPrefix(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	path := filepath.Join(ws.Root(), "deep-lines.txt")
	if err := os.WriteFile(path, []byte("aaaa\nbbbb\ncccc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := ws.OpenReadFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = readFileLinesBounded(context.Background(), file, readFileInput{
		Path: "deep-lines.txt", StartLine: 3, Limit: MaxReadFileBytes,
	}, newJSONCall(t, "read-lines-budget", "read", map[string]any{"path": "deep-lines.txt"}), 8)
	if err == nil || !strings.Contains(err.Error(), "byte offset pagination") {
		t.Fatalf("bounded line scan error = %v, want byte pagination guidance", err)
	}
}

func TestReadFileLineRangePreservesFinalLineWithoutNewline(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.WriteFile(filepath.Join(ws.Root(), "lines.txt"), []byte("a\nb"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-lines-final", "read", map[string]any{
		"path": "lines.txt", "start_line": 2, "end_line": 2,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "b" {
		t.Fatalf("Output = %q, want final line without synthesized newline", result.Output)
	}
}

func TestReadFileMissingTargetIsNotFound(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	_, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-missing", "read", map[string]any{
		"path": "worker/src/infrastructure/store.rs",
	}))
	if err == nil {
		t.Fatal("Execute() error = nil, want missing target failure")
	}
	failure := tool.FailureFromError(err)
	if failure == nil || failure.Code != tool.ErrorCodeNotFound {
		t.Fatalf("failure = %#v, want not_found", failure)
	}
	if strings.Contains(failure.Message, "execution_error") {
		t.Fatalf("missing path leaked execution_error semantics: %q", failure.Message)
	}
}

func TestReadFileDirectorySuggestsListDirRecovery(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.MkdirAll(filepath.Join(ws.Root(), "internal/base/runtimepolicy"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := New(ws).Execute(context.Background(), newJSONCall(t, "read-dir", "read", map[string]any{
		"path": "internal/base/runtimepolicy",
	}))
	if err == nil {
		t.Fatal("Execute() error = nil, want directory recovery")
	}
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("Execute() error = %T %v, want *tool.ToolError", err, err)
	}
	if toolErr.Code != tool.ErrorCodeInvalidArguments {
		t.Fatalf("error code = %q, want invalid_arguments", toolErr.Code)
	}
	if toolErr.Recovery == nil || toolErr.Recovery.Action != tool.RecoveryUseDedicatedTool || toolErr.Recovery.Tool != "ls" {
		t.Fatalf("recovery = %#v, want ls dedicated-tool recovery", toolErr.Recovery)
	}
	var args map[string]any
	if err := json.Unmarshal(toolErr.Recovery.Arguments, &args); err != nil {
		t.Fatalf("decode recovery arguments: %v", err)
	}
	if got := args["path"]; got != "internal/base/runtimepolicy" {
		t.Fatalf("recovery path = %#v", got)
	}
}
