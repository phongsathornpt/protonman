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
		"PROTONMAN_HOME=" + home,
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
	sessionFile := filepath.Join(home, ".proton", "sessions", sessionID, "state.json")
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
	if !strings.Contains(secondRes.stdout, "Hello Coding E2E") {
		t.Fatalf("output missing file content: %s", secondRes.stdout)
	}
}

func TestE2ECheckpointsAndRestore(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	// Original content of hello.txt
	origContent := "Hello Coding E2E\nLine 2\n"

	// 1. Mutate file with search_replace, using --output json to capture checkpoint_id
	srRes := runProton(t, runOptions{
		args: []string{
			"-y",
			"-p", `/call search_replace {"file_path":"hello.txt", "old_string":"Hello Coding E2E", "new_string":"Mutated Content"}`,
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

func TestE2ENewSessionByDefaultAndResume(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	// 1. Resume in a workspace with no previous session should fail
	noRes := runProton(t, runOptions{
		args: []string{"--resume", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if noRes.exitCode == 0 {
		t.Fatalf("expected --resume with no prior session to fail, got code 0; stdout: %s", noRes.stdout)
	}
	combined := noRes.stdout + noRes.stderr
	if !strings.Contains(combined, "no previous session found") {
		t.Fatalf("expected error about no previous session, got: %s", combined)
	}

	// 2. First normal run (starts clean new session)
	res1 := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if res1.exitCode != 0 {
		t.Fatalf("first run failed (code %d): %s\n%s", res1.exitCode, res1.stdout, res1.stderr)
	}

	sessDir := filepath.Join(home, ".proton", "sessions")
	files1, err := filepath.Glob(filepath.Join(sessDir, "workspace-*", "state.json"))
	if err != nil {
		t.Fatalf("glob sessions: %v", err)
	}
	if len(files1) != 1 {
		t.Fatalf("expected exactly 1 session file, got %d", len(files1))
	}
	firstSessionFile := files1[0]

	// 3. Second normal run without --resume: should create a SECOND new session file
	res2 := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if res2.exitCode != 0 {
		t.Fatalf("second run failed (code %d): %s\n%s", res2.exitCode, res2.stdout, res2.stderr)
	}

	files2, err := filepath.Glob(filepath.Join(sessDir, "workspace-*", "state.json"))
	if err != nil {
		t.Fatalf("glob sessions: %v", err)
	}
	if len(files2) != 2 {
		t.Fatalf("expected 2 distinct session files after two default runs, got %d", len(files2))
	}

	// Verify the second session file is different from the first
	hasNewFile := false
	for _, f := range files2 {
		if f != firstSessionFile {
			hasNewFile = true
			break
		}
	}
	if !hasNewFile {
		t.Fatal("expected new distinct session file to be created")
	}

	// 4. Third run with --resume: should resume the most recent session rather than creating a third
	res3 := runProton(t, runOptions{
		args: []string{"-y", "--resume", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if res3.exitCode != 0 {
		t.Fatalf("third run with --resume failed (code %d): %s\n%s", res3.exitCode, res3.stdout, res3.stderr)
	}

	files3, err := filepath.Glob(filepath.Join(sessDir, "workspace-*", "state.json"))
	if err != nil {
		t.Fatalf("glob sessions: %v", err)
	}
	if len(files3) != 2 {
		t.Fatalf("expected session count to remain 2 after --resume, got %d", len(files3))
	}

	// 5. Run with explicit --session flag
	customSess := "custom-test-session"
	res4 := runProton(t, runOptions{
		args: []string{"-y", "-s", customSess, "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if res4.exitCode != 0 {
		t.Fatalf("run with -s failed (code %d): %s\n%s", res4.exitCode, res4.stdout, res4.stderr)
	}
	customFile := filepath.Join(sessDir, customSess, "state.json")
	if _, err := os.Stat(customFile); err != nil {
		t.Fatalf("expected custom session file %s to exist: %v", customFile, err)
	}
}
