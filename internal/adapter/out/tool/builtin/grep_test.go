package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/core/workspace"
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

func TestGrepSupportsContinuationOffset(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wsDir, "many.txt"), []byte("needle 1\nneedle 2\nneedle 3\nneedle 4\nneedle 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(wsDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewGrep(ws)

	firstArgs, _ := json.Marshal(map[string]any{"pattern": "needle", "limit": 2})
	firstCall, _ := tool.NewCall("grep-page-1", "grep", firstArgs)
	first, err := handler.Execute(context.Background(), firstCall)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Truncated || first.NextOffset == nil || *first.NextOffset != 2 {
		t.Fatalf("first continuation = truncated:%v next:%v", first.Truncated, first.NextOffset)
	}
	if !strings.Contains(first.Output, "many.txt:1:needle 1") || !strings.Contains(first.Output, "many.txt:2:needle 2") {
		t.Fatalf("first output = %q", first.Output)
	}
	if strings.Contains(first.Output, "needle 3") {
		t.Fatalf("first output leaked next page: %q", first.Output)
	}

	secondArgs, _ := json.Marshal(map[string]any{"pattern": "needle", "offset": 2, "limit": 2, "continuation": first.Continuation})
	secondCall, _ := tool.NewCall("grep-page-2", "grep", secondArgs)
	second, err := handler.Execute(context.Background(), secondCall)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Truncated || second.NextOffset == nil || *second.NextOffset != 4 {
		t.Fatalf("second continuation = truncated:%v next:%v", second.Truncated, second.NextOffset)
	}
	if second.Continuation == "" || second.Continuation == first.Continuation {
		t.Fatalf("second cursor continuation = %q", second.Continuation)
	}
	if !strings.Contains(second.Output, "many.txt:3:needle 3") || !strings.Contains(second.Output, "many.txt:4:needle 4") {
		t.Fatalf("second output = %q", second.Output)
	}

	thirdArgs, _ := json.Marshal(map[string]any{"pattern": "needle", "offset": 4, "limit": 2, "continuation": second.Continuation})
	thirdCall, _ := tool.NewCall("grep-page-3", "grep", thirdArgs)
	third, err := handler.Execute(context.Background(), thirdCall)
	if err != nil {
		t.Fatal(err)
	}
	if third.Truncated || third.NextOffset != nil || !strings.Contains(third.Output, "many.txt:5:needle 5") {
		t.Fatalf("third page = %+v", third)
	}
}

func TestGrepContinuationRejectsChangedQueryButSurvivesWorkspaceMutation(t *testing.T) {
	wsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wsDir, "many.txt"), []byte("needle 1\nneedle 2\nneedle 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(wsDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewGrep(ws)
	firstArgs, _ := json.Marshal(map[string]any{"pattern": "needle", "limit": 1})
	firstCall, _ := tool.NewCall("grep-token-1", "grep", firstArgs)
	first, err := handler.Execute(context.Background(), firstCall)
	if err != nil {
		t.Fatal(err)
	}
	if first.Continuation == "" || first.NextOffset == nil {
		t.Fatalf("continuation = %q next=%v", first.Continuation, first.NextOffset)
	}

	changedQuery, _ := json.Marshal(map[string]any{"pattern": "needle 2", "offset": *first.NextOffset, "limit": 1, "continuation": first.Continuation})
	changedCall, _ := tool.NewCall("grep-token-2", "grep", changedQuery)
	_, err = handler.Execute(context.Background(), changedCall)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeStaleContinuation {
		t.Fatalf("changed query error = %v", err)
	}
	if toolErr.Recovery == nil || toolErr.Recovery.Action != tool.RecoveryRestartPagination || toolErr.Recovery.Tool != "grep" {
		t.Fatalf("changed query recovery = %#v", toolErr.Recovery)
	}

	if err := os.WriteFile(filepath.Join(wsDir, "many.txt"), []byte("needle 1\nneedle 2\nneedle 3\nneedle 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changedState, _ := json.Marshal(map[string]any{"pattern": "needle", "offset": *first.NextOffset, "limit": 1, "continuation": first.Continuation})
	stateCall, _ := tool.NewCall("grep-token-3", "grep", changedState)
	stateResult, err := handler.Execute(context.Background(), stateCall)
	if err != nil {
		t.Fatalf("changed workspace error = %v, want best-effort continuation", err)
	}
	if !strings.Contains(stateResult.Output, "needle 2") {
		t.Fatalf("changed workspace output = %q", stateResult.Output)
	}
}

func TestGrepWalkDoesNotFollowSymlinkEscapes(t *testing.T) {
	wsDir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("needle outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(wsDir, "secret-link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(wsDir, "outside-dir")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "visible.txt"), []byte("needle inside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(wsDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := executeJSON(t, NewGrep(ws), "grep-symlink", map[string]any{"pattern": "needle"})
	if !strings.Contains(result.Output, "visible.txt:1:needle inside") {
		t.Fatalf("grep output missing visible file: %q", result.Output)
	}
	if strings.Contains(result.Output, "outside") || strings.Contains(result.Output, "secret-link") {
		t.Fatalf("grep followed symlink escape: %q", result.Output)
	}
}

func TestGrepSkipsBinaryAndBuildDirectories(t *testing.T) {
	wsDir := t.TempDir()
	// Regular text file
	if err := os.WriteFile(filepath.Join(wsDir, "code.go"), []byte("match_me := 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Binary file with null bytes in root
	if err := os.WriteFile(filepath.Join(wsDir, "compiled_app"), []byte("\x7fELF\x02\x01\x01\x00match_me binary data"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Ignored build directory
	binDir := filepath.Join(wsDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "artifact.go"), []byte("match_me in bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Binary extension file
	if err := os.WriteFile(filepath.Join(wsDir, "image.png"), []byte("match_me in png\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := workspace.New(wsDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := executeJSON(t, NewGrep(ws), "grep-skip-bin", map[string]any{"pattern": "match_me"})
	if !strings.Contains(result.Output, "code.go:1:match_me := 42") {
		t.Fatalf("grep output missing text file match: %q", result.Output)
	}
	if strings.Contains(result.Output, "compiled_app") {
		t.Fatalf("grep leaked binary file: %q", result.Output)
	}
	if strings.Contains(result.Output, "bin/artifact.go") {
		t.Fatalf("grep traversed into bin/ dir: %q", result.Output)
	}
	if strings.Contains(result.Output, "image.png") {
		t.Fatalf("grep searched binary extension image.png: %q", result.Output)
	}
}
