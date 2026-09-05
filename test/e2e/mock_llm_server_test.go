package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type mockLLMResponse struct {
	status      int
	contentType string
	rawBody     string
	sseChunks   []string
}

type mockLLMServer struct {
	server           *httptest.Server
	mu               sync.Mutex
	responses        []mockLLMResponse
	receivedRequests []map[string]any
}

func newMockLLMServer(t *testing.T) *mockLLMServer {
	t.Helper()
	m := &mockLLMServer{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		bodyBytes, _ := io.ReadAll(r.Body)
		var parsedReq map[string]any
		_ = json.Unmarshal(bodyBytes, &parsedReq)
		m.receivedRequests = append(m.receivedRequests, parsedReq)

		if len(m.responses) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"delta":{"content":"Default mock response"}}]}`))
			return
		}

		resp := m.responses[0]
		m.responses = m.responses[1:]

		if resp.status == 0 {
			resp.status = http.StatusOK
		}

		if resp.contentType != "" {
			w.Header().Set("Content-Type", resp.contentType)
		} else if len(resp.sseChunks) > 0 {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}

		w.WriteHeader(resp.status)

		if resp.rawBody != "" {
			_, _ = w.Write([]byte(resp.rawBody))
			return
		}

		if len(resp.sseChunks) > 0 {
			flusher, ok := w.(http.Flusher)
			for _, chunk := range resp.sseChunks {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
				if ok {
					flusher.Flush()
				}
			}
			_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
			if ok {
				flusher.Flush()
			}
		}
	}))

	t.Cleanup(func() {
		m.server.Close()
	})

	return m
}

func (m *mockLLMServer) URL() string {
	return m.server.URL
}

func (m *mockLLMServer) AddTextResponse(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	chunk := fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, text)
	m.responses = append(m.responses, mockLLMResponse{
		status:    http.StatusOK,
		sseChunks: []string{chunk},
	})
}

func (m *mockLLMServer) AddToolCallResponse(id, name, args string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	escapedArgs, _ := json.Marshal(args)
	chunk := fmt.Sprintf(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":%q,"type":"function","function":{"name":%q,"arguments":%s}}]}}]}`, id, name, string(escapedArgs))
	m.responses = append(m.responses, mockLLMResponse{
		status:    http.StatusOK,
		sseChunks: []string{chunk},
	})
}

type mockToolCall struct {
	ID   string
	Name string
	Args string
}

func (m *mockLLMServer) AddToolCallsResponse(calls ...mockToolCall) {
	m.mu.Lock()
	defer m.mu.Unlock()

	toolCalls := make([]map[string]any, 0, len(calls))
	for index, call := range calls {
		toolCalls = append(toolCalls, map[string]any{
			"index": index,
			"id":    call.ID,
			"type":  "function",
			"function": map[string]any{
				"name":      call.Name,
				"arguments": call.Args,
			},
		})
	}
	payload, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": toolCalls},
		}},
	})
	m.responses = append(m.responses, mockLLMResponse{
		status:    http.StatusOK,
		sseChunks: []string{string(payload)},
	})
}

func (m *mockLLMServer) AddErrorResponse(status int, body string, contentType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if contentType == "" {
		contentType = "application/json"
	}
	m.responses = append(m.responses, mockLLMResponse{
		status:      status,
		contentType: contentType,
		rawBody:     body,
	})
}

func (m *mockLLMServer) AddPersistentErrorResponse(status int, body string, contentType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if contentType == "" {
		contentType = "application/json"
	}
	for i := 0; i < 5; i++ {
		m.responses = append(m.responses, mockLLMResponse{
			status:      status,
			contentType: contentType,
			rawBody:     body,
		})
	}
}

func (m *mockLLMServer) SetupWorkspaceConfig(t *testing.T, homeDir string) {
	t.Helper()
	protonDir := filepath.Join(homeDir, ".proton")
	if err := os.MkdirAll(protonDir, 0o755); err != nil {
		t.Fatalf("mkdir .proton in home: %v", err)
	}

	configTOML := fmt.Sprintf(`
[model]
default = "mock-model"
provider = "protonman"

[providers.protonman]
api_key = "mock-api-key"
base_url = "%s"
`, m.URL())

	if err := os.WriteFile(filepath.Join(protonDir, "config.toml"), []byte(configTOML), 0o644); err != nil {
		t.Fatalf("write config.toml in home: %v", err)
	}
}

func (m *mockLLMServer) Requests() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()

	requests := make([]map[string]any, 0, len(m.receivedRequests))
	for _, request := range m.receivedRequests {
		copyRequest := make(map[string]any, len(request))
		for key, value := range request {
			copyRequest[key] = value
		}
		requests = append(requests, copyRequest)
	}
	return requests
}
