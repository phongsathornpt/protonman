package builtin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin/readfile"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestReadFilePublishesSHA256ForCompleteSnapshot(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "file.txt", "hello\n")
	result := executeJSON(t, readfile.New(ws), "read-hash", map[string]any{"path": "file.txt"})
	digest := sha256.Sum256([]byte("hello\n"))
	if want := fmt.Sprintf("%x", digest[:]); result.SHA256 != want {
		t.Fatalf("read sha256 = %q, want %q", result.SHA256, want)
	}
}

func TestReadFileOmitsSHA256ForPartialSnapshot(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "file.txt", strings.Repeat("a", 64))
	result := executeJSON(t, readfile.New(ws), "read-partial", map[string]any{"path": "file.txt", "limit": 8})
	if !result.Truncated || result.SHA256 != "" {
		t.Fatalf("partial read truncated=%v sha256=%q", result.Truncated, result.SHA256)
	}
}
func TestWriteFileRequiresExpectedSHA256ForOverwrite(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "file.txt", "old\n")
	_, err := NewWriteFile(ws, &recordingCheckpointStore{id: "hash"}).Execute(context.Background(),
		newJSONCall(t, "write-missing-hash", "edit", map[string]any{"file_path": "file.txt", "content": "new\n"}))
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeInvalidArguments {
		t.Fatalf("missing hash error = %v, want invalid arguments", err)
	}
	if toolErr.Recovery == nil || toolErr.Recovery.Action != tool.RecoveryRefreshResource || toolErr.Recovery.Tool != tool.NameRead {
		t.Fatalf("missing hash recovery = %#v", toolErr.Recovery)
	}
	var recoveryArgs map[string]any
	if err := json.Unmarshal(toolErr.Recovery.Arguments, &recoveryArgs); err != nil || recoveryArgs["path"] != "file.txt" {
		t.Fatalf("missing hash recovery args = %s err=%v", toolErr.Recovery.Arguments, err)
	}
}

func TestWriteFileRejectsStaleExpectedSHA256(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "file.txt", "current\n")
	stale := sha256.Sum256([]byte("stale\n"))
	_, err := NewWriteFile(ws, &recordingCheckpointStore{id: "hash"}).Execute(context.Background(),
		newJSONCall(t, "write-stale-hash", "edit", map[string]any{
			"file_path": "file.txt", "content": "new\n", "expected_sha256": fmt.Sprintf("%x", stale[:]),
		}))
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeConflict {
		t.Fatalf("stale hash error = %v, want conflict", err)
	}
	if toolErr.Recovery == nil || toolErr.Recovery.Action != tool.RecoveryRefreshResource || toolErr.Recovery.Tool != "read" {
		t.Fatalf("stale hash recovery = %#v", toolErr.Recovery)
	}
	var recoveryArgs map[string]any
	if err := json.Unmarshal(toolErr.Recovery.Arguments, &recoveryArgs); err != nil || recoveryArgs["path"] != "file.txt" {
		t.Fatalf("stale hash recovery args = %s err=%v", toolErr.Recovery.Arguments, err)
	}
	if got := string(readTestFile(t, ws.Root(), "file.txt")); got != "current\n" {
		t.Fatalf("stale write changed file: %q", got)
	}
}
func TestWriteFileAcceptsMatchingExpectedSHA256(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "file.txt", "old\n")
	old := sha256.Sum256([]byte("old\n"))
	result := executeJSON(t, NewWriteFile(ws, &recordingCheckpointStore{id: "hash"}), "write-good-hash", map[string]any{
		"file_path": "file.txt", "content": "new\n", "expected_sha256": fmt.Sprintf("%x", old[:]),
	})
	newDigest := sha256.Sum256([]byte("new\n"))
	if want := fmt.Sprintf("%x", newDigest[:]); result.SHA256 != want {
		t.Fatalf("write sha256 = %q, want %q", result.SHA256, want)
	}
	if got := string(readTestFile(t, ws.Root(), "file.txt")); got != "new\n" {
		t.Fatalf("updated file = %q", got)
	}
}

func TestWriteFileCreateDoesNotRequireExpectedSHA256(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	result := executeJSON(t, NewWriteFile(ws, &recordingCheckpointStore{id: "hash"}), "write-new", map[string]any{
		"file_path": "new.txt", "content": "new\n",
	})
	if result.SHA256 == "" {
		t.Fatal("new file result missing sha256")
	}
}
