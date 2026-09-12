package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin/readfile"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
)

func BenchmarkReadFile64KB(b *testing.B) {
	dir := b.TempDir()
	ws, err := workspace.New(dir, nil)
	if err != nil {
		b.Fatalf("workspace.New: %v", err)
	}
	handler := readfile.New(ws)
	content := strings.Repeat("a", 64*1024)
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0o644); err != nil {
		b.Fatalf("WriteFile: %v", err)
	}

	call := tool.Call{
		ID:        "bench-1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"test.txt"}`),
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, err := handler.Execute(ctx, call)
		if err != nil {
			b.Fatalf("Execute: %v", err)
		}
	}
}

func BenchmarkGrep100Files(b *testing.B) {
	dir := b.TempDir()
	ws, err := workspace.New(dir, nil)
	if err != nil {
		b.Fatalf("workspace.New: %v", err)
	}
	handler := NewGrep(ws)

	// Create 100 files with 50 lines each
	for i := 0; i < 100; i++ {
		var lines []string
		for j := 0; j < 50; j++ {
			if j == 25 && i%10 == 0 {
				lines = append(lines, fmt.Sprintf("target match in file %d line %d", i, j))
			} else {
				lines = append(lines, fmt.Sprintf("filler content for file %d line %d regular text", i, j))
			}
		}
		fname := filepath.Join(dir, fmt.Sprintf("file_%03d.txt", i))
		_ = os.WriteFile(fname, []byte(strings.Join(lines, "\n")), 0o644)
	}

	call := tool.Call{
		ID:        "bench-grep",
		Name:      "grep",
		Arguments: json.RawMessage(`{"pattern":"target match"}`),
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		res, err := handler.Execute(ctx, call)
		if err != nil {
			b.Fatalf("grep: %v", err)
		}
		if res.Output == "" {
			b.Fatal("grep expected matches")
		}
	}
}

func BenchmarkListDir500Entries(b *testing.B) {
	dir := b.TempDir()
	ws, err := workspace.New(dir, nil)
	if err != nil {
		b.Fatalf("workspace.New: %v", err)
	}
	handler := NewListDir(ws)

	for i := 0; i < 500; i++ {
		fname := filepath.Join(dir, fmt.Sprintf("entry_%03d.txt", i))
		_ = os.WriteFile(fname, []byte("data"), 0o644)
	}

	call := tool.Call{
		ID:        "bench-listdir",
		Name:      "ls",
		Arguments: json.RawMessage(`{"path":"."}`),
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, err := handler.Execute(ctx, call)
		if err != nil {
			b.Fatalf("ls: %v", err)
		}
	}
}
