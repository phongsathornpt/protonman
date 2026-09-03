package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ESessionPersistenceAndRedaction(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	sessionID := "e2e-test-session"
	env := []string{
		"PROTON_HOME=" + home,
		"PROTON_SESSION_ID=" + sessionID,
	}

	// First run with -y (sets mode to always-approve) and calls read_file
	firstRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if firstRes.exitCode != 0 {
		t.Fatalf("first run failed (code %d): %s\n%s", firstRes.exitCode, firstRes.stdout, firstRes.stderr)
	}

	// Verify session file was saved
	sessionFile := filepath.Join(home, ".proton", "sessions", sessionID+".json")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("session file not found at %s: %v", sessionFile, err)
	}

	var sessionData struct {
		PermissionMode string `json:"permission_mode"`
		Messages       []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolName   string `json:"tool_name"`
			ToolCallID string `json:"tool_call_id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &sessionData); err != nil {
		t.Fatalf("decode session file: %v", err)
	}

	if sessionData.PermissionMode != "always-approve" {
		t.Fatalf("saved permission mode = %q, want always-approve", sessionData.PermissionMode)
	}

	// Verify transcript was saved and tool arguments were redacted
	if len(sessionData.Messages) == 0 {
		t.Fatal("expected saved messages in session")
	}
	for _, msg := range sessionData.Messages {
		if strings.Contains(msg.Content, "hello.txt") {
			t.Fatalf("session leaked tool arguments in transcript: %+v", msg)
		}
	}

	// Second run: do NOT pass -y. Since session restored mode always-approve, this should succeed!
	secondRes := runProton(t, runOptions{
		args: []string{"-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if secondRes.exitCode != 0 {
		t.Fatalf("second run failed to restore always-approve mode from session (code %d): %s\n%s",
			secondRes.exitCode, secondRes.stdout, secondRes.stderr)
	}
	if !strings.Contains(secondRes.stdout, "Hello Proton E2E") {
		t.Fatalf("output missing file content: %s", secondRes.stdout)
	}
}

func TestE2ECheckpointsAndRestore(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTON_HOME=" + home}

	// Original content of hello.txt
	origContent := "Hello Proton E2E\nLine 2\n"

	// 1. Mutate file with search_replace, using --output json to capture checkpoint_id
	srRes := runProton(t, runOptions{
		args: []string{
			"-y",
			"-p", `/call search_replace {"file_path":"hello.txt", "old_string":"Hello Proton E2E", "new_string":"Mutated Content"}`,
			"--output", "json",
		},
		dir: ws,
		env: env,
	})
	if srRes.exitCode != 0 {
		t.Fatalf("search_replace failed: %s\n%s", srRes.stdout, srRes.stderr)
	}

	// Find the created checkpoint file on disk under .proton/checkpoints/
	checkpointFiles, err := filepath.Glob(filepath.Join(home, ".proton", "checkpoints", "*", "checkpoint-*.json"))
	if err != nil || len(checkpointFiles) == 0 {
		t.Fatalf("no checkpoint file found in %s: %v", filepath.Join(home, ".proton", "checkpoints"), err)
	}
	checkpointID := strings.TrimSuffix(filepath.Base(checkpointFiles[0]), ".json")
	if checkpointID == "" {
		t.Fatal("empty checkpoint ID")
	}

	// Verify file was mutated on disk
	diskContent, err := os.ReadFile(filepath.Join(ws, "hello.txt"))
	if err != nil {
		t.Fatalf("read mutated file: %v", err)
	}
	if !strings.Contains(string(diskContent), "Mutated Content") {
		t.Fatalf("file not mutated: %s", string(diskContent))
	}

	// 2. Restore checkpoint via /call checkpoint_restore
	restoreRes := runProton(t, runOptions{
		args: []string{
			"-y",
			"-p", `/call checkpoint_restore {"checkpoint_id":"` + checkpointID + `"}`,
		},
		dir: ws,
		env: env,
	})
	if restoreRes.exitCode != 0 {
		t.Fatalf("checkpoint_restore failed: %s\n%s", restoreRes.stdout, restoreRes.stderr)
	}

	// 3. Verify file on disk is restored to original content
	restoredContent, err := os.ReadFile(filepath.Join(ws, "hello.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(restoredContent) != origContent {
		t.Fatalf("file not restored: got %q, want %q", string(restoredContent), origContent)
	}
}
