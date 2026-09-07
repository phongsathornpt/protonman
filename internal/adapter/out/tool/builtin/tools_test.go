package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	skilltool "github.com/projectTHORN/proton/internal/adapter/out/tool/skill"
	todotool "github.com/projectTHORN/proton/internal/adapter/out/tool/todo"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/core/workspace"
	"github.com/projectTHORN/proton/internal/feature/agent"
)

func testSandboxOption() RegistryOption {
	return WithSandbox(&recordingLauncher{})
}

func testCheckpointOption() RegistryOption {
	return WithCheckpointStore(&recordingCheckpointStore{id: "test"})
}

func TestWriteFileAndSearchReplace(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeHandler := NewWriteFile(workspaceRoot, &recordingCheckpointStore{id: "test"})
	replaceHandler := NewSearchReplace(workspaceRoot, &recordingCheckpointStore{id: "test"})

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
		{name: "write protected", handler: NewWriteFile(workspaceRoot, &recordingCheckpointStore{id: "x"}), path: ".env", wantErr: workspace.ErrProtectedPath},
		{name: "read protected", handler: NewReadFile(workspaceRoot), path: ".env", wantErr: workspace.ErrProtectedPath},
		{
			name:    "write traversal",
			handler: NewWriteFile(workspaceRoot, &recordingCheckpointStore{id: "x"}),
			path:    "../outside.txt",
			wantErr: workspace.ErrOutsideWorkspace,
		},
		{
			name:    "read traversal",
			handler: NewReadFile(workspaceRoot),
			path:    "../outside.txt",
			wantErr: workspace.ErrOutsideWorkspace,
		},
		{
			name:    "read protected symlink",
			handler: NewReadFile(workspaceRoot),
			path:    "linked-secret.txt",
			wantErr: workspace.ErrProtectedPath,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.handler.Execute(
				context.Background(),
				newJSONCall(t, test.name, test.handler.Definition().Name, map[string]any{
					"file_path": test.path,
					"path":      test.path,
					"content":   "should not be written",
				}),
			)
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
	if result.NextOffset == nil || *result.NextOffset != int64(maxReadFileBytes) {
		t.Fatalf("read next_offset = %v, want %d", result.NextOffset, maxReadFileBytes)
	}
	marker := fmt.Sprintf("\n[output truncated; continue with offset=%d]", maxReadFileBytes)
	if got, want := len(result.Output), maxReadFileBytes+len(marker); got != want {
		t.Fatalf("read output length = %d, want %d", got, want)
	}
}

func TestReadFileSupportsContinuationOffset(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeTestFile(t, workspaceRoot.Root(), "paged.txt", "abcdefghij")
	handler := NewReadFile(workspaceRoot)

	first := executeJSON(t, handler, "read-page-1", map[string]any{"path": "paged.txt", "limit": 4})
	if !first.Truncated || first.NextOffset == nil || *first.NextOffset != 4 {
		t.Fatalf("first page continuation = truncated:%v next:%v", first.Truncated, first.NextOffset)
	}
	if !strings.HasPrefix(first.Output, "abcd") {
		t.Fatalf("first page output = %q", first.Output)
	}

	second := executeJSON(t, handler, "read-page-2", map[string]any{"path": "paged.txt", "offset": 4, "limit": 4})
	if !second.Truncated || second.NextOffset == nil || *second.NextOffset != 8 {
		t.Fatalf("second page continuation = truncated:%v next:%v", second.Truncated, second.NextOffset)
	}
	if !strings.HasPrefix(second.Output, "efgh") {
		t.Fatalf("second page output = %q", second.Output)
	}

	third := executeJSON(t, handler, "read-page-3", map[string]any{"path": "paged.txt", "offset": 8, "limit": 4})
	if third.Truncated || third.NextOffset != nil || third.Output != "ij" {
		t.Fatalf("third page = %+v", third)
	}
}

func TestReadFileRejectsDirectory(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeTestFile(t, workspaceRoot.Root(), "sub/file.txt", "content")

	_, err := NewReadFile(workspaceRoot).Execute(
		context.Background(),
		newJSONCall(t, "read-dir", "read_file", map[string]any{"path": "sub"}),
	)
	if err == nil {
		t.Fatal("expected error reading directory, got nil")
	}
	if !strings.Contains(err.Error(), "is a directory; use list_dir instead") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestApplyPatchSupportsFileOperationsAndPlansBeforeWriting(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeTestFile(t, workspaceRoot.Root(), "before.txt", "one\ntwo\n")
	writeTestFile(t, workspaceRoot.Root(), "remove.txt", "remove me\n")
	handler := NewApplyPatch(workspaceRoot, &recordingCheckpointStore{id: "test"})

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
	registry, err := NewDefaultRegistry(workspaceRoot, testSandboxOption(), testCheckpointOption())
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	definitions := registry.Definitions()
	if got, want := len(definitions), 11; got != want {
		t.Fatalf("definition count = %d, want %d", got, want)
	}

	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	regWithCoord, err := NewDefaultRegistry(workspaceRoot, testSandboxOption(), testCheckpointOption(), withAgentTools(coord))
	if err != nil {
		t.Fatalf("NewDefaultRegistry(WithAgentCoordinator) error = %v", err)
	}
	if got, want := len(regWithCoord.Definitions()), 16; got != want {
		t.Fatalf("definition count with coordinator = %d, want %d", got, want)
	}
	for _, name := range []string{"delegate_task", "wait_agent", "get_agent", "list_agents", "cancel_agent"} {
		if _, ok := regWithCoord.Lookup(name); !ok {
			t.Fatalf("%s not found in registry", name)
		}
	}
}

func TestBuiltinInputSchemasRejectUnknownProperties(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	registry, err := NewDefaultRegistry(workspaceRoot, testSandboxOption(), testCheckpointOption(), withAgentTools(coord))
	if err != nil {
		t.Fatal(err)
	}
	definitions := registry.Definitions()
	definitions = append(definitions,
		todotool.NewGetTodo(nil).Definition(),
		todotool.NewUpdateTodo(nil).Definition(),
		skilltool.NewActivateSkill(nil, workspaceRoot).Definition(),
	)
	for _, definition := range definitions {
		if len(definition.InputSchema) == 0 {
			continue
		}
		got, exists := definition.InputSchema["additionalProperties"]
		if !exists || got != false {
			t.Errorf("%s InputSchema additionalProperties = %#v, want false", definition.Name, got)
		}
	}
}

func TestWriteFilePublishesCheckpointID(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	checkpointStore := &recordingCheckpointStore{id: "checkpoint-test"}
	result := executeJSON(t, NewWriteFile(workspaceRoot, checkpointStore), "write-checkpoint", map[string]any{
		"file_path": "checkpointed.txt",
		"content":   "checkpoint me\n",
	})
	if result.CheckpointID != checkpointStore.id {
		t.Fatalf("checkpoint ID = %q, want %q", result.CheckpointID, checkpointStore.id)
	}
	if result.MutationCoverage != tool.MutationCoverageFull {
		t.Fatalf("mutation coverage = %q, want full", result.MutationCoverage)
	}
	if len(result.AffectedPaths) != 1 || result.AffectedPaths[0] != "checkpointed.txt" {
		t.Fatalf("affected paths = %#v, want checkpointed.txt", result.AffectedPaths)
	}
	wantPath := filepath.Join(workspaceRoot.Root(), "checkpointed.txt")
	if len(checkpointStore.paths) != 1 || checkpointStore.paths[0] != wantPath {
		t.Fatalf("checkpoint paths = %#v, want target path", checkpointStore.paths)
	}

	restoreResult := executeJSON(t, NewCheckpointRestore(checkpointStore), "restore-checkpoint", map[string]any{
		"checkpoint_id": checkpointStore.id,
	})
	if !strings.Contains(restoreResult.Output, checkpointStore.id) {
		t.Fatalf("restore output = %q, want checkpoint ID", restoreResult.Output)
	}
	if checkpointStore.restored != checkpointStore.id {
		t.Fatalf("restored checkpoint = %q, want %q", checkpointStore.restored, checkpointStore.id)
	}
}

func TestGitStatusReportsWorkspaceRepository(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	gitCommand := exec.Command("git", "init", "--quiet", workspaceRoot.Root())
	if output, err := gitCommand.CombinedOutput(); err != nil {
		t.Fatalf("git init error = %v, output = %s", err, output)
	}
	writeTestFile(t, workspaceRoot.Root(), "status.txt", "changed\n")

	result := executeJSON(t, NewGitStatus(workspaceRoot, &recordingLauncher{}), "status-1", map[string]any{})
	if !strings.Contains(result.Output, "status.txt") {
		t.Fatalf("git status output = %q, want status.txt", result.Output)
	}
	if !strings.Contains(result.Output, "??") {
		t.Fatalf("git status output = %q, want untracked marker", result.Output)
	}
}

func TestGitStatusRejectsNonRepository(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	_, err := NewGitStatus(workspaceRoot, &recordingLauncher{}).Execute(
		context.Background(),
		newJSONCall(t, "status-2", "git_status", map[string]any{}),
	)
	if err == nil {
		t.Fatal("git_status error = nil, want non-repository error")
	}
}

func TestGitStatusBoundedBufferCapsMemoryAndSignalsLimit(t *testing.T) {
	limited := false
	buffer := &boundedBuffer{limit: 8, onLimit: func() { limited = true }}
	if _, err := buffer.Write([]byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if got := buffer.String(); got != "12345678" {
		t.Fatalf("bounded buffer = %q, want first 8 bytes", got)
	}
	if !buffer.IsTruncated() || !limited {
		t.Fatal("bounded buffer did not report output limit")
	}
}

func TestGitStatusCancelsWhenStdoutExceedsLimit(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	_, err := NewGitStatus(workspaceRoot, scriptedGitLauncher{script: "yes x | head -c 2097152"}).Execute(
		context.Background(),
		newJSONCall(t, "status-large", "git_status", map[string]any{}),
	)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeOutputTooLarge {
		t.Fatalf("git_status large output error = %v, want output_too_large", err)
	}
}

func TestGitStatusIncludesBoundedStderrDiagnostic(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	_, err := NewGitStatus(workspaceRoot, scriptedGitLauncher{script: "printf 'not a git repository' >&2; exit 128"}).Execute(
		context.Background(),
		newJSONCall(t, "status-stderr", "git_status", map[string]any{}),
	)
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("git_status stderr diagnostic = %v", err)
	}
}

func TestGitStatusRequiresLauncherFailClosed(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	_, err := NewGitStatus(workspaceRoot).Execute(
		context.Background(),
		newJSONCall(t, "status-nil", "git_status", map[string]any{}),
	)
	if err == nil {
		t.Fatal("git_status error = nil, want launcher-required error")
	}
}

func TestDefaultRegistryRequiresSandboxAndCheckpointFailClosed(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	if _, err := NewDefaultRegistry(workspaceRoot); err == nil {
		t.Fatal("NewDefaultRegistry() error = nil, want sandbox+checkpoint required error")
	}
	if _, err := NewDefaultRegistry(workspaceRoot, testCheckpointOption()); err == nil {
		t.Fatal("NewDefaultRegistry(checkpoint only) error = nil, want sandbox required error")
	}
	if _, err := NewDefaultRegistry(workspaceRoot, testSandboxOption()); err == nil {
		t.Fatal("NewDefaultRegistry(sandbox only) error = nil, want checkpoint required error")
	}
}

func TestWriteWithoutCheckpointFailsClosed(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	_, err := NewWriteFile(workspaceRoot).Execute(
		context.Background(),
		newJSONCall(t, "write-nostore", "write_file", map[string]any{
			"file_path": "nostore.txt",
			"content":   "should not be written",
		}),
	)
	if err == nil {
		t.Fatal("write_file without store error = nil, want checkpoint error")
	}
}

type mockGitLauncher struct {
	lastDir     string
	lastCommand string
}

type scriptedGitLauncher struct {
	script string
}

func (l scriptedGitLauncher) Command(ctx context.Context, dir string, _ string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", l.script)
	cmd.Dir = dir
	return cmd, nil
}

func (m *mockGitLauncher) Command(_ context.Context, dir string, command string) (*exec.Cmd, error) {
	m.lastDir = dir
	m.lastCommand = command
	cmd := exec.Command("echo", "## main")
	cmd.Dir = dir
	return cmd, nil
}

func TestGitStatusUsesLauncher(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	launcher := &mockGitLauncher{}
	result, err := NewGitStatus(workspaceRoot, launcher).Execute(
		context.Background(),
		newJSONCall(t, "status-launcher", "git_status", map[string]any{"path": "sub"}),
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "## main") {
		t.Fatalf("output = %q, want ## main", result.Output)
	}
	if launcher.lastDir != workspaceRoot.Root() {
		t.Fatalf("launcher dir = %q, want %q", launcher.lastDir, workspaceRoot.Root())
	}
	if !strings.Contains(launcher.lastCommand, "core.hooksPath=/dev/null") {
		t.Fatalf("command = %q, want core.hooksPath=/dev/null", launcher.lastCommand)
	}
	if !strings.Contains(launcher.lastCommand, "--no-optional-locks") {
		t.Fatalf("command = %q, want --no-optional-locks", launcher.lastCommand)
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

type recordingCheckpointStore struct {
	id       string
	paths    []string
	restored string
}

func (s *recordingCheckpointStore) Capture(_ context.Context, paths []string) (string, error) {
	s.paths = append([]string{}, paths...)
	return s.id, nil
}

func (s *recordingCheckpointStore) Restore(_ context.Context, id string) error {
	s.restored = id
	return nil
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

func TestPermissionDetailProviders(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)

	// Test apply_patch detail extraction
	patchTool := NewApplyPatch(workspaceRoot, &recordingCheckpointStore{id: "detail"})
	detailedPatch, ok := patchTool.(tool.DetailProvider)
	if !ok {
		t.Fatal("apply_patch does not implement tool.DetailProvider")
	}
	patchPayload := `*** Begin Patch
*** Add File: pkg/math.go
+package pkg
*** End Patch`
	args, _ := json.Marshal(map[string]any{"patch": patchPayload})
	if detail := detailedPatch.PermissionDetail(args); detail != "add 1 · pkg/math.go" {
		t.Fatalf("apply_patch PermissionDetail = %q, want add summary", detail)
	}

	// Test list_dir detail extraction with aliases and default
	listTool := NewListDir(workspaceRoot)
	detailedList, ok := listTool.(tool.DetailProvider)
	if !ok {
		t.Fatal("list_dir does not implement tool.DetailProvider")
	}
	argsDir, _ := json.Marshal(map[string]any{"dir_path": "src/lib"})
	if detail := detailedList.PermissionDetail(argsDir); detail != "src/lib" {
		t.Fatalf("list_dir with dir_path PermissionDetail = %q, want src/lib", detail)
	}
	argsEmpty, _ := json.Marshal(map[string]any{})
	if detail := detailedList.PermissionDetail(argsEmpty); detail != "." {
		t.Fatalf("list_dir empty PermissionDetail = %q, want .", detail)
	}

	// Test grep detail extraction default
	grepTool := NewGrep(workspaceRoot)
	detailedGrep, ok := grepTool.(tool.DetailProvider)
	if !ok {
		t.Fatal("grep does not implement tool.DetailProvider")
	}
	argsGrep, _ := json.Marshal(map[string]any{"pattern": "TODO"})
	if detail := detailedGrep.PermissionDetail(argsGrep); detail != "." {
		t.Fatalf("grep empty path PermissionDetail = %q, want .", detail)
	}
	argsGrepPath, _ := json.Marshal(map[string]any{"pattern": "TODO", "path": "docs"})
	if detail := detailedGrep.PermissionDetail(argsGrepPath); detail != "docs" {
		t.Fatalf("grep with path PermissionDetail = %q, want docs", detail)
	}
}

func TestReadFilePaginationPreservesUTF8Boundaries(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeTestFile(t, workspaceRoot.Root(), "utf8.txt", "A界B")
	handler := NewReadFile(workspaceRoot)

	first := executeJSON(t, handler, "utf8-page-1", map[string]any{"path": "utf8.txt", "limit": 2})
	if !first.Truncated || first.NextOffset == nil || *first.NextOffset != 1 {
		t.Fatalf("first continuation = truncated:%v next:%v", first.Truncated, first.NextOffset)
	}
	if !strings.HasPrefix(first.Output, "A\n[output truncated") {
		t.Fatalf("first page = %q", first.Output)
	}

	second := executeJSON(t, handler, "utf8-page-2", map[string]any{"path": "utf8.txt", "offset": 1, "limit": 2})
	if !second.Truncated || second.NextOffset == nil || *second.NextOffset != 4 {
		t.Fatalf("second continuation = truncated:%v next:%v", second.Truncated, second.NextOffset)
	}
	if !strings.HasPrefix(second.Output, "界\n[output truncated") {
		t.Fatalf("second page = %q", second.Output)
	}

	third := executeJSON(t, handler, "utf8-page-3", map[string]any{"path": "utf8.txt", "offset": 4, "limit": 2})
	if third.Truncated || third.NextOffset != nil || third.Output != "B" {
		t.Fatalf("third page = %+v", third)
	}
}

func TestReadFileRejectsOffsetInsideUTF8CodePoint(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeTestFile(t, workspaceRoot.Root(), "utf8.txt", "A界B")
	_, err := NewReadFile(workspaceRoot).Execute(
		context.Background(),
		newJSONCall(t, "utf8-split", "read_file", map[string]any{"path": "utf8.txt", "offset": 2, "limit": 2}),
	)
	if err == nil || !strings.Contains(err.Error(), "splits a UTF-8 code point") {
		t.Fatalf("Execute() error = %v, want UTF-8 boundary rejection", err)
	}
}

func TestReadFileContinuationRejectsChangedFile(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	writeTestFile(t, workspaceRoot.Root(), "snapshot.txt", "abcdefghij")
	handler := NewReadFile(workspaceRoot)
	first := executeJSON(t, handler, "snapshot-1", map[string]any{"path": "snapshot.txt", "limit": 4})
	if first.Continuation == "" || first.NextOffset == nil {
		t.Fatalf("first continuation = %q next=%v", first.Continuation, first.NextOffset)
	}
	writeTestFile(t, workspaceRoot.Root(), "snapshot.txt", "abcdefghij changed")
	_, err := handler.Execute(context.Background(), newJSONCall(t, "snapshot-2", "read_file", map[string]any{
		"path": "snapshot.txt", "offset": *first.NextOffset, "limit": 4, "continuation": first.Continuation,
	}))
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeStaleContinuation {
		t.Fatalf("Execute() error = %v, want stale continuation", err)
	}
}

func TestWorkspaceObservationToolsDeclareGroundingEvidence(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	for _, handler := range []tool.Handler{NewReadFile(ws), NewListDir(ws), NewGrep(ws)} {
		if got := handler.Definition().Evidence; got != tool.EvidenceWorkspace {
			t.Fatalf("%s evidence = %q, want workspace", handler.Definition().Name, got)
		}
	}
}
