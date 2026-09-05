package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

func BenchmarkGrepLiteralMedium(b *testing.B) {
	benchmarkGrep(b, "needle", "", 5000, 20)
}

func BenchmarkGrepRegexMedium(b *testing.B) {
	benchmarkGrep(b, `needle [0-9]+`, "", 5000, 20)
}

func BenchmarkGrepIncludeMedium(b *testing.B) {
	benchmarkGrep(b, "needle", "*.go", 5000, 20)
}

func BenchmarkGrepDeepPageMedium(b *testing.B) {
	root := benchmarkWorkspace(b, 5000, 20)
	ws, err := workspace.New(root, nil)
	if err != nil {
		b.Fatal(err)
	}
	handler := NewGrep(ws)
	args, _ := json.Marshal(map[string]any{"pattern": "needle", "offset": 90, "limit": 10})
	call, _ := tool.NewCall("bench", "grep", args)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := handler.Execute(context.Background(), call); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkGrep(b *testing.B, pattern, include string, files, lines int) {
	root := benchmarkWorkspace(b, files, lines)
	ws, err := workspace.New(root, nil)
	if err != nil {
		b.Fatal(err)
	}
	handler := NewGrep(ws)
	input := map[string]any{"pattern": pattern, "limit": 10}
	if include != "" {
		input["include"] = include
	}
	args, _ := json.Marshal(input)
	call, _ := tool.NewCall("bench", "grep", args)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := handler.Execute(context.Background(), call); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkWorkspace(b *testing.B, files, lines int) string {
	b.Helper()
	root := b.TempDir()
	for i := 0; i < files; i++ {
		dir := filepath.Join(root, fmt.Sprintf("d%02d", i%32))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		ext := ".txt"
		if i%4 == 0 {
			ext = ".go"
		}
		path := filepath.Join(dir, fmt.Sprintf("f%05d%s", i, ext))
		f, err := os.Create(path)
		if err != nil {
			b.Fatal(err)
		}
		for line := 0; line < lines; line++ {
			text := fmt.Sprintf("ordinary line %d file %d\n", line, i)
			if line == lines-1 && i%50 == 0 {
				text = fmt.Sprintf("needle %d\n", i)
			}
			if _, err := f.WriteString(text); err != nil {
				_ = f.Close()
				b.Fatal(err)
			}
		}
		if err := f.Close(); err != nil {
			b.Fatal(err)
		}
	}
	return root
}
