package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestE2EACPErrorsAndMethods(t *testing.T) {
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
	}()

	reader := bufio.NewReader(stdoutPipe)

	send := func(line string) {
		if _, err := io.WriteString(stdinPipe, line+"\n"); err != nil {
			t.Fatalf("write to acp stdin: %v", err)
		}
	}

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

	// 1. Unknown method (-32601)
	send(`{"jsonrpc":"2.0","id":100,"method":"non_existent_method_xyz","params":{}}`)
	resp100 := readResponse(100)
	if errObj, ok := resp100["error"].(map[string]any); !ok {
		t.Fatalf("expected error object for unknown method, got: %+v", resp100)
	} else if code, ok := errObj["code"].(float64); !ok || (int(code) != -32601 && int(code) != -32000) {
		t.Fatalf("expected error code, got: %v", errObj["code"])
	}

	// 2. session/prompt on non-existent session
	send(`{"jsonrpc":"2.0","id":101,"method":"session/prompt","params":{"sessionId":"non-existent-session-id","prompt":[{"type":"text","text":"hello"}]}}`)
	resp101 := readResponse(101)
	if errObj, ok := resp101["error"].(map[string]any); !ok {
		t.Fatalf("expected error object for missing session, got: %+v", resp101)
	} else if msg, ok := errObj["message"].(string); !ok || (!strings.Contains(msg, "not found") && !strings.Contains(msg, "unknown session")) {
		t.Fatalf("expected session not found message, got: %v", errObj["message"])
	}
}
