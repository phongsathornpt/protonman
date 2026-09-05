package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

func TestGrepTool(t *testing.T) {
	ctx := context.Background()
	wsDir := t.TempDir()

	// Setup directory structure:
	// wsDir/
	//   main.go (content: "func main() { hello() }")
	//   readme.md (content: "hello world")
	//   cmd/
	//     app.go (content: "package main\n// hello from cmd")
	//   sub/
	//     helper.go (content: "func hello() {}\n")
	//     data.txt (content: "hello in data")

	if err := os.WriteFile(filepath.Join(wsDir, "main.go"), []byte("func main() { hello() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "readme.md"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wsDir, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "cmd", "app.go"), []byte("package main\n// hello from cmd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wsDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "sub", "helper.go"), []byte("func hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "sub", "data.txt"), []byte("hello in data\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := workspace.New(wsDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewGrep(ws)

	t.Run("basic search matches all files containing pattern", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"pattern": "hello",
		})
		call, _ := tool.NewCall("grep-1", "grep", args)
		res, err := handler.Execute(ctx, call)
		if err != nil || res.Failure != nil {
			t.Fatalf("unexpected error: %v, failure: %+v", err, res.Failure)
		}
		if !strings.Contains(res.Output, "main.go:1:") {
			t.Errorf("expected main.go in output, got: %s", res.Output)
		}
		if !strings.Contains(res.Output, "readme.md:1:") {
			t.Errorf("expected readme.md in output, got: %s", res.Output)
		}
		if !strings.Contains(res.Output, "cmd/app.go:2:") {
			t.Errorf("expected cmd/app.go in output, got: %s", res.Output)
		}
		if !strings.Contains(res.Output, "sub/helper.go:1:") {
			t.Errorf("expected sub/helper.go in output, got: %s", res.Output)
		}
	})

	t.Run("include filename glob *.go filters by extension", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"pattern": "hello",
			"include": "*.go",
		})
		call, _ := tool.NewCall("grep-2", "grep", args)
		res, err := handler.Execute(ctx, call)
		if err != nil || res.Failure != nil {
			t.Fatalf("unexpected error: %v, failure: %+v", err, res.Failure)
		}
		if !strings.Contains(res.Output, "main.go:1:") {
			t.Errorf("expected main.go in output, got: %s", res.Output)
		}
		if !strings.Contains(res.Output, "cmd/app.go:2:") {
			t.Errorf("expected cmd/app.go in output, got: %s", res.Output)
		}
		if strings.Contains(res.Output, "readme.md") {
			t.Errorf("did not expect readme.md in output, got: %s", res.Output)
		}
		if strings.Contains(res.Output, "data.txt") {
			t.Errorf("did not expect data.txt in output, got: %s", res.Output)
		}
	})

	t.Run("include path glob sub/*.go matches files in subdirectory", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"pattern": "hello",
			"include": "sub/*.go",
		})
		call, _ := tool.NewCall("grep-3", "grep", args)
		res, err := handler.Execute(ctx, call)
		if err != nil || res.Failure != nil {
			t.Fatalf("unexpected error: %v, failure: %+v", err, res.Failure)
		}
		if !strings.Contains(res.Output, "sub/helper.go:1:") {
			t.Errorf("expected sub/helper.go in output, got: %s", res.Output)
		}
		if strings.Contains(res.Output, "main.go") {
			t.Errorf("did not expect main.go with sub/*.go glob, got: %s", res.Output)
		}
		if strings.Contains(res.Output, "cmd/app.go") {
			t.Errorf("did not expect cmd/app.go with sub/*.go glob, got: %s", res.Output)
		}
	})

	t.Run("include doublestar glob **/*.go matches root and nested files", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"pattern": "hello",
			"include": "**/*.go",
		})
		call, _ := tool.NewCall("grep-4", "grep", args)
		res, err := handler.Execute(ctx, call)
		if err != nil || res.Failure != nil {
			t.Fatalf("unexpected error: %v, failure: %+v", err, res.Failure)
		}
		if !strings.Contains(res.Output, "main.go:1:") {
			t.Errorf("expected main.go in output, got: %s", res.Output)
		}
		if !strings.Contains(res.Output, "cmd/app.go:2:") {
			t.Errorf("expected cmd/app.go in output, got: %s", res.Output)
		}
		if !strings.Contains(res.Output, "sub/helper.go:1:") {
			t.Errorf("expected sub/helper.go in output, got: %s", res.Output)
		}
		if strings.Contains(res.Output, "readme.md") {
			t.Errorf("did not expect readme.md in output, got: %s", res.Output)
		}
	})

	t.Run("invalid regex pattern returns error", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"pattern": "[unclosed",
		})
		call, _ := tool.NewCall("grep-5", "grep", args)
		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for invalid regex, got nil")
		}
	})

	t.Run("path restriction searches only within given directory", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"pattern": "hello",
			"path":    "cmd",
		})
		call, _ := tool.NewCall("grep-6", "grep", args)
		res, err := handler.Execute(ctx, call)
		if err != nil || res.Failure != nil {
			t.Fatalf("unexpected error: %v, failure: %+v", err, res.Failure)
		}
		if !strings.Contains(res.Output, "cmd/app.go:2:") {
			t.Errorf("expected cmd/app.go in output, got: %s", res.Output)
		}
		if strings.Contains(res.Output, "main.go") {
			t.Errorf("did not expect main.go when restricted to cmd, got: %s", res.Output)
		}
	})
}
