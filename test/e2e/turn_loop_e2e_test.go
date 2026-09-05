package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ETurnLoopSingleTurnText(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)
	server.AddTextResponse("Hello from mock Protonman agent!")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Say hello to me"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("turn loop failed (code %d): %s %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "Hello from mock Protonman agent!") {
		t.Fatalf("stdout missing agent response: %s", res.stdout)
	}
}

func TestE2ETurnLoopToolCallAndResultCycle(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)

	// Round 1: Model responds with a tool call to write a file
	server.AddToolCallResponse("call_write_1", "write_file", `{"file_path":"agent_output.txt","content":"Created by AI Agent"}`)
	// Round 2: Model confirms after tool execution
	server.AddTextResponse("File has been successfully created.")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Create agent_output.txt"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("tool call loop failed (code %d): %s %s", res.exitCode, res.stdout, res.stderr)
	}

	// Verify file was written to disk by Proton's tool executor
	diskContent, err := os.ReadFile(filepath.Join(ws, "agent_output.txt"))
	if err != nil {
		t.Fatalf("agent_output.txt not created: %v", err)
	}
	if string(diskContent) != "Created by AI Agent" {
		t.Fatalf("agent_output.txt content = %q, want 'Created by AI Agent'", string(diskContent))
	}

	if !strings.Contains(res.stdout, "File has been successfully created.") {
		t.Fatalf("stdout missing agent confirmation: %s", res.stdout)
	}
}

func TestE2ETurnLoopMultiRoundChain(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)

	// Round 1: list_dir
	server.AddToolCallResponse("call_list_1", "list_dir", `{"path":"."}`)
	// Round 2: read_file
	server.AddToolCallResponse("call_read_2", "read_file", `{"path":"hello.txt"}`)
	// Round 3: final answer
	server.AddTextResponse("The file contains Hello Proton E2E.")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Inspect workspace and report content"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("multi-round chain failed (code %d): %s %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "The file contains Hello Proton E2E.") {
		t.Fatalf("stdout missing final summary: %s", res.stdout)
	}
}

func TestE2ETurnLoopSessionResumption(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)

	// Turn 1
	server.AddTextResponse("I remember that secret is purple-elephant.")
	res1 := runProton(t, runOptions{
		args: []string{"-y", "-p", "Remember the secret purple-elephant"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res1.exitCode != 0 || !strings.Contains(res1.stdout, "purple-elephant") {
		t.Fatalf("turn 1 failed: %s %s", res1.stdout, res1.stderr)
	}

	// Turn 2 with --resume
	server.AddTextResponse("Yes, the secret was purple-elephant!")
	res2 := runProton(t, runOptions{
		args: []string{"-y", "-r", "-p", "What was the secret?"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res2.exitCode != 0 || !strings.Contains(res2.stdout, "purple-elephant") {
		t.Fatalf("turn 2 resume failed: %s %s", res2.stdout, res2.stderr)
	}
}
