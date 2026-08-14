package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/adapters/workspace"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

func TestWriteFileAndSearchReplace(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeHandler := NewWriteFile(workspaceRoot)
	replaceHandler := NewSearchReplace(workspaceRoot)

	result := executeJSON(t, writeHandler, "write-1", map[string]any{
		"file_path": "notes.txt",
		"content":   "hello\nhello\n",
	})
	if !strings.Contains(result.Output, "Wrote file successfully") {
		t.Fatalf("write output = %q", result.Output)
	}

	_, err := replaceHandler.Execute(context.Background(), newJSONCall(t, "replace-1", "search_replace", map[string]any{
		"file_path":  "notes.txt",
		"old_string": "hello",
		"new_string": "goodbye",
	}))
	if err == nil || !strings.Contains(err.Error(), "replace_all") {
		t.Fatalf("multiple replacement error = %v, want replace_all guidance", err)
	}

	executeJSON(t, replaceHandler, "replace-2", map[string]any{
		"file_path":   "notes.txt",
		"old_string":  "hello",
		"new_string":  "goodbye",
		"replace_all": true,
	})
	contents, err := os.ReadFile(filepath.Join(workspaceRoot.Root(), "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(contents), "goodbye\ngoodbye\n"; got != want {
		t.Fatalf("updated contents = %q, want %q", got, want)
	}
}

func TestFileToolsRejectTraversalAndProtectedPaths(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, []string{".env", "secrets"})
	if err := os.WriteFile(filepath.Join(workspaceRoot.Root(), ".env"), []byte("TOKEN=hidden"), 0o600); err != nil {
		t.Fatalf("write protected file: %v", err)
	}
	protectedTarget := filepath.Join(workspaceRoot.Root(), "secrets", "linked.txt")
	writeTestFile(t, workspaceRoot.Root(), "secrets/linked.txt", "protected target")
	if err := os.Symlink(protectedTarget, filepath.Join(workspaceRoot.Root(), "linked-secret.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	for _, test := range []struct {
		name    string
		handler tool.Handler
		path    string
		wantErr error
	}{
		{name: "write protected", handler: NewWriteFile(workspaceRoot), path: ".env", wantErr: workspace.ErrProtectedPath},
		{name: "read protected", handler: NewReadFile(workspaceRoot), path: ".env", wantErr: workspace.ErrProtectedPath},
		{name: "write traversal", handler: NewWriteFile(workspaceRoot), path: "../outside.txt", wantErr: workspace.ErrOutsideWorkspace},
		{name: "read traversal", handler: NewReadFile(workspaceRoot), path: "../outside.txt", wantErr: workspace.ErrOutsideWorkspace},
		{name: "read protected symlink", handler: NewReadFile(workspaceRoot), path: "linked-secret.txt", wantErr: workspace.ErrProtectedPath},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.handler.Execute(context.Background(), newJSONCall(t, test.name, test.handler.Definition().Name, map[string]any{
				"file_path": test.path,
				"path":      test.path,
				"content":   "should not be written",
			}))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, test.wantErr)
			}
		})
	}
}

func TestGrepAndListDirHideProtectedEntries(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, []string{".env", "secrets", "certs"})
	writeTestFile(t, workspaceRoot.Root(), "main.go", "needle visible\n")
	writeTestFile(t, workspaceRoot.Root(), ".env", "needle hidden\n")
	writeTestFile(t, workspaceRoot.Root(), "secrets/token.txt", "needle hidden\n")
	writeTestFile(t, workspaceRoot.Root(), "certs/private.pem", "needle hidden\n")

	grepResult := executeJSON(t, NewGrep(workspaceRoot), "grep-1", map[string]any{
		"pattern": "needle",
	})
	if !strings.Contains(grepResult.Output, "main.go:1:needle visible") {
		t.Fatalf("grep output = %q, want visible match", grepResult.Output)
	}
	if strings.Contains(grepResult.Output, "hidden") {
		t.Fatalf("grep output exposed protected content: %q", grepResult.Output)
	}

	listResult := executeJSON(t, NewListDir(workspaceRoot), "list-1", map[string]any{"path": "."})
	if !strings.Contains(listResult.Output, "main.go") {
		t.Fatalf("list output = %q, want main.go", listResult.Output)
	}
	for _, protectedEntry := range []string{".env", "secrets", "certs"} {
		if strings.Contains(listResult.Output, protectedEntry) {
			t.Fatalf("list output exposed protected entry %q: %q", protectedEntry, listResult.Output)
		}
	}
}

func TestGrepReportsTruncation(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	var content strings.Builder
	for index := 0; index < maxGrepResults+1; index++ {
		content.WriteString("needle ")
		content.WriteString(strings.Repeat("x", 20))
		content.WriteByte('\n')
	}
	writeTestFile(t, workspaceRoot.Root(), "many.txt", content.String())

	result := executeJSON(t, NewGrep(workspaceRoot), "grep-limit", map[string]any{"pattern": "needle"})
	if !result.Truncated {
		t.Fatal("grep result Truncated = false, want true")
	}
	if !strings.Contains(result.Output, "output truncated") {
		t.Fatalf("grep output = %q, want truncation marker", result.Output)
	}
}

func TestReadFileReportsTruncation(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	contents := strings.Repeat("x", maxReadFileBytes+1)
	writeTestFile(t, workspaceRoot.Root(), "large.txt", contents)

	result := executeJSON(t, NewReadFile(workspaceRoot), "read-limit", map[string]any{"path": "large.txt"})
	if !result.Truncated {
		t.Fatal("read result Truncated = false, want true")
	}
	if !strings.Contains(result.Output, "output truncated") {
		t.Fatalf("read output does not contain truncation marker")
	}
	if got, want := len(result.Output), maxReadFileBytes+len("\n[output truncated at 2 MiB]"); got != want {
		t.Fatalf("read output length = %d, want %d", got, want)
	}
}

func TestApplyPatchSupportsFileOperationsAndPlansBeforeWriting(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeTestFile(t, workspaceRoot.Root(), "before.txt", "one\ntwo\n")
	writeTestFile(t, workspaceRoot.Root(), "remove.txt", "remove me\n")
	handler := NewApplyPatch(workspaceRoot)

	patch := "*** Begin Patch\n" +
		"*** Add File: added.txt\n" +
		"+created\n" +
		"*** Update File: before.txt\n" +
		"@@\n" +
		" one\n" +
		"-two\n" +
		"+TWO\n" +
		"*** Delete File: remove.txt\n" +
		"*** End Patch"
	executeJSON(t, handler, "patch-1", map[string]any{"patch": patch})

	if got := string(readTestFile(t, workspaceRoot.Root(), "added.txt")); got != "created\n" {
		t.Fatalf("added contents = %q", got)
	}
	if got := string(readTestFile(t, workspaceRoot.Root(), "before.txt")); got != "one\nTWO\n" {
		t.Fatalf("updated contents = %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot.Root(), "remove.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed file stat error = %v, want not-exist", err)
	}

	writeTestFile(t, workspaceRoot.Root(), "move.txt", "move me\n")
	movePatch := "*** Begin Patch\n" +
		"*** Update File: move.txt\n" +
		"*** Move to: moved.txt\n" +
		"@@\n" +
		" move me\n" +
		"+now moved\n" +
		"*** End Patch"
	executeJSON(t, handler, "patch-2", map[string]any{"patch": movePatch})
	if _, err := os.Stat(filepath.Join(workspaceRoot.Root(), "move.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source stat error = %v, want not-exist", err)
	}
	if got := string(readTestFile(t, workspaceRoot.Root(), "moved.txt")); got != "move me\nnow moved\n" {
		t.Fatalf("moved contents = %q", got)
	}

	badPatch := "*** Begin Patch\n" +
		"*** Add File: should-not-exist.txt\n" +
		"+not written\n" +
		"*** Update File: before.txt\n" +
		"@@\n" +
		"-missing\n" +
		"+bad\n" +
		"*** End Patch"
	_, err := handler.Execute(context.Background(), newJSONCall(t, "patch-3", "apply_patch", map[string]any{"patch": badPatch}))
	if err == nil {
		t.Fatal("bad patch error = nil, want planning error")
	}
	if _, statErr := os.Stat(filepath.Join(workspaceRoot.Root(), "should-not-exist.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("bad patch created file, stat error = %v", statErr)
	}
}

func TestDefaultRegistryContainsCodingTools(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	registry, err := NewDefaultRegistry(workspaceRoot)
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	definitions := registry.Definitions()
	if got, want := len(definitions), 8; got != want {
		t.Fatalf("definition count = %d, want %d", got, want)
	}
}

func TestGitStatusReportsWorkspaceRepository(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	gitCommand := exec.Command("git", "init", "--quiet", workspaceRoot.Root())
	if output, err := gitCommand.CombinedOutput(); err != nil {
		t.Fatalf("git init error = %v, output = %s", err, output)
	}
	writeTestFile(t, workspaceRoot.Root(), "status.txt", "changed\n")

	result := executeJSON(t, NewGitStatus(workspaceRoot), "status-1", map[string]any{})
	if !strings.Contains(result.Output, "status.txt") {
		t.Fatalf("git status output = %q, want status.txt", result.Output)
	}
	if !strings.Contains(result.Output, "??") {
		t.Fatalf("git status output = %q, want untracked marker", result.Output)
	}
}

func TestGitStatusRejectsNonRepository(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	_, err := NewGitStatus(workspaceRoot).Execute(
		context.Background(),
		newJSONCall(t, "status-2", "git_status", map[string]any{}),
	)
	if err == nil {
		t.Fatal("git_status error = nil, want non-repository error")
	}
}

func newTestWorkspace(t *testing.T, protected []string) *workspace.Workspace {
	t.Helper()
	root := t.TempDir()
	workspaceRoot, err := workspace.New(root, protected)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	return workspaceRoot
}

func writeTestFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func readTestFile(t *testing.T, root string, relativePath string) []byte {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return contents
}

func executeJSON(t *testing.T, handler tool.Handler, id string, input map[string]any) tool.Result {
	t.Helper()
	result, err := handler.Execute(context.Background(), newJSONCall(t, id, handler.Definition().Name, input))
	if err != nil {
		t.Fatalf("%s.Execute() error = %v", handler.Definition().Name, err)
	}
	return result
}

func newJSONCall(t *testing.T, id string, name string, input map[string]any) tool.Call {
	t.Helper()
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	call, err := tool.NewCall(id, name, arguments)
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	return call
}
