package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirEmptyDirectory(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	emptyDir := filepath.Join(ws.Root(), "empty_folder")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	handler := NewListDir(ws)
	result := executeJSON(t, handler, "list-empty", map[string]any{"path": "empty_folder"})
	if !strings.Contains(result.Output, "(empty directory)") {
		t.Fatalf("expected '(empty directory)', got: %q", result.Output)
	}
}

func TestListDirParameterAliases(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	subDir := filepath.Join(ws.Root(), "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, ws.Root(), "sub/nested.txt", "content")

	handler := NewListDir(ws)

	// 1. dir_path alias
	res1 := executeJSON(t, handler, "list-alias-1", map[string]any{"dir_path": "sub"})
	if !strings.Contains(res1.Output, "nested.txt") {
		t.Fatalf("dir_path alias failed: %s", res1.Output)
	}

	// 2. directory alias
	res2 := executeJSON(t, handler, "list-alias-2", map[string]any{"directory": "sub"})
	if !strings.Contains(res2.Output, "nested.txt") {
		t.Fatalf("directory alias failed: %s", res2.Output)
	}
}

func TestListDirSymlinkTarget(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "target.txt", "hello")
	subDir := filepath.Join(ws.Root(), "target_dir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fileLink := filepath.Join(ws.Root(), "link_to_file")
	if err := os.Symlink("target.txt", fileLink); err != nil {
		t.Skip("symlinks not supported")
	}
	dirLink := filepath.Join(ws.Root(), "link_to_dir")
	if err := os.Symlink("target_dir", dirLink); err != nil {
		t.Skip("symlinks not supported")
	}

	handler := NewListDir(ws)
	result := executeJSON(t, handler, "list-symlinks", map[string]any{"path": "."})

	// File symlink
	if !strings.Contains(result.Output, "link link_to_file -> target.txt") {
		t.Fatalf("missing file symlink target, got:\n%s", result.Output)
	}
	// Directory symlink with trailing slash
	if !strings.Contains(result.Output, "link link_to_dir/ -> target_dir/") {
		t.Fatalf("missing directory symlink target with slash, got:\n%s", result.Output)
	}
}

func TestListDirFileSizeFormatting(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "small.txt", "hello") // 5 bytes
	largeBytes := make([]byte, 1024*1024+100)         // ~1.0 MiB
	writeTestFile(t, ws.Root(), "large.bin", string(largeBytes))

	handler := NewListDir(ws)
	result := executeJSON(t, handler, "list-sizes", map[string]any{"path": "."})

	// Small file: exact bytes
	if !strings.Contains(result.Output, "file small.txt (5 bytes)") {
		t.Fatalf("expected exact 5 bytes for small file, got:\n%s", result.Output)
	}
	// Large file: human readable unit + bytes
	if !strings.Contains(result.Output, "file large.bin (1.0 MiB, ") {
		t.Fatalf("expected 1.0 MiB formatted size for large file, got:\n%s", result.Output)
	}
}

func TestListDirAccurateTruncationWithProtectedEntries(t *testing.T) {
	// Protected paths that are skipped should not prematurely truncate visible items
	protected := []string{"secret1", "secret2", "secret3"}
	ws := newTestWorkspace(t, protected)
	for _, p := range protected {
		writeTestFile(t, ws.Root(), p, "secret")
	}
	writeTestFile(t, ws.Root(), "visible.txt", "visible")

	handler := NewListDir(ws)
	result := executeJSON(t, handler, "list-trunc", map[string]any{"path": "."})

	if result.Truncated {
		t.Fatal("unexpected truncation")
	}
	if !strings.Contains(result.Output, "visible.txt") {
		t.Fatalf("expected visible.txt, got:\n%s", result.Output)
	}
	for _, p := range protected {
		if strings.Contains(result.Output, p) {
			t.Fatalf("protected entry %s leaked in output", p)
		}
	}
}
