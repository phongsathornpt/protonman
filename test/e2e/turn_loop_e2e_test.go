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

func TestE2ETurnLoopForcesSynthesisAfterRepeatedRead(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)
	server.AddToolCallResponse("call_read_loop_1", "read_file", `{"path":"hello.txt"}`)
	server.AddToolCallResponse("call_read_loop_2", "read_file", `{"path":"hello.txt"}`)
	server.AddTextResponse("I already have enough information from the repeated read.")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Read hello.txt until you can answer"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("semantic loop synthesis failed (code %d): %s %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "I already have enough information") {
		t.Fatalf("stdout missing forced synthesis: %s", res.stdout)
	}

	requests := server.Requests()
	if got, want := len(requests), 3; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	if requestToolCount(requests[0]) == 0 || requestToolCount(requests[1]) == 0 {
		t.Fatalf("tool-enabled requests unexpectedly omitted tools: %#v", requests)
	}
	if got := requestToolCount(requests[2]); got != 0 {
		t.Fatalf("forced synthesis request tools = %d, want 0", got)
	}
	if !requestMessagesContain(requests[2], "TOOL LOOP DETECTED") {
		t.Fatalf("forced synthesis request missing no-progress prompt: %#v", requests[2]["messages"])
	}
}

func TestE2ETurnLoopIgnoresRepeatedToolAfterLoopDetected(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)
	server.AddToolCallResponse("call_read_ignore_1", "read_file", `{"path":"hello.txt"}`)
	server.AddToolCallResponse("call_read_ignore_2", "read_file", `{"path":"hello.txt"}`)
	server.AddToolCallResponse("call_read_ignore_3", "read_file", `{"path":"hello.txt"}`)

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Keep reading hello.txt"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("ignored loop call failed (code %d): %s %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "stopped a repeated tool loop") {
		t.Fatalf("stdout missing loop fallback: %s", res.stdout)
	}
	requests := server.Requests()
	if got, want := len(requests), 3; got != want {
		t.Fatalf("model requests = %d, want %d; provider should not receive a fourth retry", got, want)
	}
	if got := requestToolCount(requests[2]); got != 0 {
		t.Fatalf("no-progress request tools = %d, want 0", got)
	}
}

func TestE2EResumeCompactsHistoricalToolProtocol(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)
	server.AddToolCallResponse("call_resume_read", "read_file", `{"path":"hello.txt"}`)
	server.AddTextResponse("I inspected hello.txt.")

	res1 := runProton(t, runOptions{
		args: []string{"-y", "-p", "Inspect hello.txt"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res1.exitCode != 0 {
		t.Fatalf("initial tool turn failed: %s %s", res1.stdout, res1.stderr)
	}

	server.AddTextResponse("The resumed history is structurally safe.")
	res2 := runProton(t, runOptions{
		args: []string{"-y", "-r", "-p", "What did you inspect?"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res2.exitCode != 0 {
		t.Fatalf("resumed tool turn failed: %s %s", res2.stdout, res2.stderr)
	}

	requests := server.Requests()
	if got, want := len(requests), 3; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	messages := requestMessages(t, requests[2])
	for _, message := range messages {
		if role, _ := message["role"].(string); role == "tool" {
			t.Fatalf("resumed request contains orphan tool role: %#v", message)
		}
		if calls, exists := message["tool_calls"]; exists && calls != nil {
			if list, ok := calls.([]any); !ok || len(list) > 0 {
				t.Fatalf("resumed request contains fabricated tool calls: %#v", message)
			}
		}
	}
	if !requestMessagesContain(requests[2], "Historical tool read_file result") {
		t.Fatalf("resumed request missing compacted historical result: %#v", requests[2]["messages"])
	}
}

func requestToolCount(request map[string]any) int {
	tools, ok := request["tools"].([]any)
	if !ok {
		return 0
	}
	return len(tools)
}

func requestMessages(t *testing.T, request map[string]any) []map[string]any {
	t.Helper()
	rawMessages, ok := request["messages"].([]any)
	if !ok {
		t.Fatalf("request messages = %#v, want array", request["messages"])
	}
	messages := make([]map[string]any, 0, len(rawMessages))
	for _, raw := range rawMessages {
		message, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("request message = %#v, want object", raw)
		}
		messages = append(messages, message)
	}
	return messages
}

func requestMessagesContain(request map[string]any, needle string) bool {
	rawMessages, ok := request["messages"].([]any)
	if !ok {
		return false
	}
	for _, raw := range rawMessages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		content, _ := message["content"].(string)
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}

func TestE2ETurnLoopSuppressesDeadCallInsideProductiveBatch(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	if err := os.WriteFile(filepath.Join(ws, "second.txt"), []byte("second file\n"), 0o644); err != nil {
		t.Fatalf("write second.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws, "third.txt"), []byte("third file\n"), 0o644); err != nil {
		t.Fatalf("write third.txt: %v", err)
	}

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)
	server.AddToolCallsResponse(
		mockToolCall{ID: "dead-1", Name: "read_file", Args: `{"path":"hello.txt"}`},
		mockToolCall{ID: "live-1", Name: "read_file", Args: `{"path":"second.txt"}`},
	)
	server.AddToolCallsResponse(
		mockToolCall{ID: "dead-2", Name: "read_file", Args: `{"path":"hello.txt"}`},
		mockToolCall{ID: "live-2", Name: "read_file", Args: `{"path":"third.txt"}`},
	)
	server.AddToolCallsResponse(
		mockToolCall{ID: "dead-3", Name: "read_file", Args: `{"path":"hello.txt"}`},
		mockToolCall{ID: "live-3", Name: "read_file", Args: `{"path":"second.txt","offset":1}`},
	)
	server.AddTextResponse("The productive branch completed while the repeated read was suppressed.")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Inspect several files without repeating dead work"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("mixed batch loop failed (code %d): %s %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "productive branch completed") {
		t.Fatalf("stdout missing final response: %s", res.stdout)
	}

	requests := server.Requests()
	if got, want := len(requests), 4; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	if !requestMessagesContain(requests[3], `"code":"no_progress"`) {
		t.Fatalf("final request missing suppressed no-progress result: %#v", requests[3]["messages"])
	}
	if !requestMessagesContain(requests[3], "econd file") {
		t.Fatalf("final request missing productive live result: %#v", requests[3]["messages"])
	}
}
