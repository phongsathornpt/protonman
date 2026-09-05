package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestE2EACPServerSessionFlow(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, protonBin, "--acp", "-y")
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PROTON_HOME="+home)
	if coverDir != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+coverDir)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start proton --acp: %v", err)
	}
	defer func() {
		_ = stdinPipe.Close()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("proton stderr:\n%s", stderrBuf.String())
		}
	}()

	reader := bufio.NewReader(stdoutPipe)

	// Helper to send a line
	send := func(line string) {
		if _, err := io.WriteString(stdinPipe, line+"\n"); err != nil {
			t.Fatalf("write to acp stdin: %v", err)
		}
	}

	// Helper to read until a response with matching ID
	readResponse := func(targetID int) map[string]any {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("read from acp stdout: %v", err)
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var msg map[string]any
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			if id, ok := msg["id"].(float64); ok && int(id) == targetID {
				return msg
			}
		}
	}

	// 1. Send initialize
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	initResp := readResponse(1)
	result, ok := initResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize response missing result: %+v", initResp)
	}
	if v, ok := result["protocolVersion"].(float64); !ok || int(v) != 1 {
		t.Fatalf("initialize protocolVersion = %v, want 1", result["protocolVersion"])
	}

	// 2. Send session/new
	send(`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{}}`)
	newResp := readResponse(2)
	newResult, ok := newResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("session/new response missing result: %+v", newResp)
	}
	sessionID, ok := newResult["sessionId"].(string)
	if !ok || sessionID == "" {
		t.Fatalf("session/new missing sessionId: %+v", newResult)
	}

	// 3. Send session/prompt calling read_file
	promptReq := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":%q,"prompt":[{"type":"text","text":"/call read_file {\"path\":\"hello.txt\"}"}]}}`,
		sessionID,
	)
	send(promptReq)
	promptResp := readResponse(3)
	promptResult, ok := promptResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("session/prompt missing result: %+v", promptResp)
	}
	if stopReason, ok := promptResult["stopReason"].(string); !ok || (stopReason != "end_turn" && stopReason != "completed") {
		t.Fatalf("session/prompt unexpected stopReason: %v", promptResult["stopReason"])
	}

	// 4. Send session/prompt calling /tools
	toolsReq := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":4,"method":"session/prompt","params":{"sessionId":%q,"prompt":[{"type":"text","text":"/tools"}]}}`,
		sessionID,
	)
	send(toolsReq)
	toolsResp := readResponse(4)
	if toolsResult, ok := toolsResp["result"].(map[string]any); !ok {
		t.Fatalf("session/prompt /tools missing result: %+v", toolsResp)
	} else if stopReason, ok := toolsResult["stopReason"].(string); !ok || (stopReason != "end_turn" && stopReason != "completed") {
		t.Fatalf("session/prompt /tools unexpected stopReason: %v", toolsResult["stopReason"])
	}
}

func TestE2EACPCancellation(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, protonBin, "--acp", "-y")
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PROTON_HOME="+home)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start proton --acp: %v", err)
	}
	defer func() {
		_ = stdinPipe.Close()
		_ = cmd.Wait()
	}()

	reader := bufio.NewReader(stdoutPipe)

	var mu sync.Mutex
	send := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = io.WriteString(stdinPipe, line+"\n")
	}

	readResponse := func(targetID int) map[string]any {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("read stdout error: %v", err)
			}
			line = strings.TrimSpace(line)
			var msg map[string]any
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			if id, ok := msg["id"].(float64); ok && int(id) == targetID {
				return msg
			}
		}
	}

	// Initialize
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	_ = readResponse(1)

	// New session
	send(`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{}}`)
	newResp := readResponse(2)
	sessionID := newResp["result"].(map[string]any)["sessionId"].(string)

	// Send slow command prompt: sleep 3
	promptReq := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":%q,"prompt":[{"type":"text","text":"/call bash {\"command\":\"sleep 3\"}"}]}}`,
		sessionID,
	)
	send(promptReq)

	// Allow bash to start running
	time.Sleep(100 * time.Millisecond)

	// Send session/cancel
	cancelReq := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":%q}}`,
		sessionID,
	)
	send(cancelReq)

	// Response for id 3 should return stopReason: "cancelled"
	promptResp := readResponse(3)
	result := promptResp["result"].(map[string]any)
	if stopReason, ok := result["stopReason"].(string); !ok || stopReason != "cancelled" {
		t.Fatalf("expected stopReason cancelled, got: %v", result["stopReason"])
	}
}
