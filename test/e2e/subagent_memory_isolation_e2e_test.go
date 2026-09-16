package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/session"
)

// memoryIsolationServer routes responses by request content so an interleaved
// root/child conversation stays deterministic without a global response queue.
type memoryIsolationServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	root     int
	child    int
	received []map[string]any
}

func newMemoryIsolationServer(t *testing.T) *memoryIsolationServer {
	t.Helper()
	m := &memoryIsolationServer{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		text := string(body)

		m.mu.Lock()
		m.received = append(m.received, request)
		switch {
		case strings.Contains(text, "Task: Inspect hello.txt"):
			m.child++
			first := m.child == 1
			m.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			if first {
				writeSSEChunk(w, toolCallChunk("child-read-1", "read", `{"path":"hello.txt"}`))
			} else {
				writeSSEChunk(w, textChunk("Child inspected hello.txt."))
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		default:
			m.root++
			first := m.root == 1
			m.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			if first {
				writeSSEChunk(w, toolCallChunk("delegate-1", "subagent", `{"action":"spawn","task":"Inspect hello.txt","profile":"agility","timeoutSeconds":30}`))
			} else {
				writeSSEChunk(w, textChunk("Delegation complete from automatic child result."))
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
	}))
	t.Cleanup(m.server.Close)
	return m
}

func (m *memoryIsolationServer) URL() string { return m.server.URL }

func (m *memoryIsolationServer) Requests() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]map[string]any, len(m.received))
	copy(out, m.received)
	return out
}

func writeSSEChunk(w http.ResponseWriter, chunk string) {
	fmt.Fprintf(w, "data: %s\n\n", chunk)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func textChunk(text string) string {
	return fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, text)
}

func toolCallChunk(id, name, args string) string {
	escapedArgs, _ := json.Marshal(args)
	return fmt.Sprintf(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":%q,"type":"function","function":{"name":%q,"arguments":%s}}]}}]}`, id, name, string(escapedArgs))
}

// TestE2ESubagentDoesNotInheritRootMemoryContext is the regression guard for
// durable-memory isolation. With no explicit subagent model override, a child
// inherits the root session's model; that inherited model must be the
// undecorated base, so child turns never receive root memory context.
func TestE2ESubagentDoesNotInheritRootMemoryContext(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	server := newMemoryIsolationServer(t)

	// Seed workspace-scoped durable memory containing a distinctive value that
	// only a memory-decorated request could contain.
	const memoryValue = "distinctive-memory-value-xyz"
	writeWorkspaceMemory(t, home, ws, memoryValue)
	writeHomeConfig(t, home, server.URL())

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Delegate inspection of hello.txt and report the result"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home, "PROTONMAN_TRUST_PROJECT=1"},
	})
	if res.exitCode != 0 {
		t.Fatalf("proton failed: stdout=%s stderr=%s", res.stdout, res.stderr)
	}

	requests := server.Requests()
	if len(requests) < 2 {
		t.Fatalf("requests = %d, want at least 2", len(requests))
	}
	// Child turns are identified by their task user message, which is stable
	// regardless of which model they inherit.
	const childMarker = "Task: Inspect hello.txt"
	var sawChild, sawRootMemory bool
	for _, request := range requests {
		isChild := requestMessagesContain(request, childMarker)
		for _, message := range requestMessages(t, request) {
			content, _ := message["content"].(string)
			switch {
			case isChild:
				sawChild = true
				if strings.Contains(content, memoryValue) {
					t.Fatalf("child request inherited root memory value: %s", content)
				}
				if strings.Contains(content, "<proton-memory-context>") {
					t.Fatalf("child request contained a memory block: %s", content)
				}
			default:
				if strings.Contains(content, memoryValue) {
					sawRootMemory = true
				}
			}
		}
	}
	if !sawChild {
		t.Fatalf("no child request observed; requests=%#v", requests)
	}
	if !sawRootMemory {
		t.Fatal("root request did not receive seeded memory; isolation assertion would be vacuous")
	}
}

// writeWorkspaceMemory seeds the workspace-scoped durable memory index used by
// the running binary. It mirrors the on-disk layout owned by the memoryfs
// adapter so the test exercises real retrieval persistence.
func writeWorkspaceMemory(t *testing.T, home, ws, value string) {
	t.Helper()
	key := session.WorkspaceKey(ws)
	dir := filepath.Join(home, ".protonman", "memory", "v1", "workspaces", key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create memory dir: %v", err)
	}
	index := fmt.Sprintf(`{
  "version": 1,
  "entries": [
    {
      "id": "mem_e2e_1",
      "scope": "workspace",
      "kind": "procedure",
      "key": "hello verification workflow",
      "value": "%s",
      "keywords": ["hello", "inspection"],
      "workspace_key": "%s",
      "confidence": 1,
      "created_at": "2026-09-01T00:00:00Z",
      "updated_at": "2026-09-01T00:00:00Z"
    }
  ]
}`, value, key)
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(index), 0o600); err != nil {
		t.Fatalf("write memory index: %v", err)
	}
}

// writeHomeConfig writes the layered provider config the running binary loads.
func writeHomeConfig(t *testing.T, home, baseURL string) {
	t.Helper()
	protonDir := filepath.Join(home, ".protonman")
	if err := os.MkdirAll(protonDir, 0o755); err != nil {
		t.Fatalf("create home config dir: %v", err)
	}
	configJSON := fmt.Sprintf(`{
  "model": {
    "default": "mock-model",
    "provider": "protonman"
  },
  "providers": {
    "protonman": {
      "api_key": "mock-api-key",
      "base_url": "%s"
    }
  }
}`, baseURL)
	if err := os.WriteFile(filepath.Join(protonDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatalf("write home config: %v", err)
	}
}
