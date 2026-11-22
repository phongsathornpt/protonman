package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	questiontool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/question"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
)

type testRegistrar struct {
	mu       sync.Mutex
	handlers map[string]tool.Handler
}

func newTestRegistrar(handlers ...tool.Handler) *testRegistrar {
	r := &testRegistrar{handlers: make(map[string]tool.Handler)}
	for _, h := range handlers {
		r.handlers[h.Definition().Name] = h
	}
	return r
}

func (r *testRegistrar) Register(h tool.Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[h.Definition().Name] = h
	return nil
}

func (r *testRegistrar) Lookup(name string) (tool.Handler, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.handlers[name]
	return h, ok
}

func (r *testRegistrar) Definitions() []tool.Definition {
	r.mu.Lock()
	defer r.mu.Unlock()
	defs := make([]tool.Definition, 0, len(r.handlers))
	for _, h := range r.handlers {
		defs = append(defs, h.Definition())
	}
	return defs
}

type scriptedQuestionClient struct {
	mu            sync.Mutex
	requests      []RequestQuestionParams
	requestsSeen  chan struct{}
	respondWith   RequestQuestionResult
	respondErr    *RPCError
	sessionIDChan chan string
}

func newScriptedQuestionClient(result RequestQuestionResult) *scriptedQuestionClient {
	return &scriptedQuestionClient{
		requestsSeen:  make(chan struct{}, 8),
		respondWith:   result,
		sessionIDChan: make(chan string, 1),
	}
}

func TestQuestionBrokerRoundTrip(t *testing.T) {
	client := newScriptedQuestionClient(RequestQuestionResult{
		Status:          questiontool.StatusAnswered,
		Answer:          "Option A",
		SelectedOptions: []string{"Option A"},
	})

	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()
	defer func() {
		_ = serverReader.Close()
		_ = clientWriter.Close()
		_ = clientReader.Close()
		_ = serverWriter.Close()
	}()

	registry := newTestRegistrar()
	policy, err := permission.NewPolicy(permission.Config{Default: permission.ActionAllow})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	service, err := toolcall.NewService(registry, policy)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	server, err := New(service, registry, nil)
	if err != nil {
		t.Fatalf("New server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Serve(ctx, serverReader, serverWriter)
	}()

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		scanner := bufio.NewScanner(clientReader)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var req RPCRequest
			if err := json.Unmarshal(line, &req); err != nil {
				continue
			}
			if req.Method == requestQuestionMethod {
				var params RequestQuestionParams
				_ = json.Unmarshal(req.Params, &params)
				client.mu.Lock()
				client.requests = append(client.requests, params)
				client.mu.Unlock()
				select {
				case client.requestsSeen <- struct{}{}:
				default:
				}

				resp := RPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result:  client.respondWith,
					Error:   client.respondErr,
				}
				data, _ := json.Marshal(resp)
				_, _ = clientWriter.Write(append(data, '\n'))
			}
		}
	}()

	// Create session through session/new
	sessionNewFrame := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/tmp/test"}}` + "\n"
	if _, err := clientWriter.Write([]byte(sessionNewFrame)); err != nil {
		t.Fatalf("write session/new: %v", err)
	}

	// Wait briefly for session creation
	time.Sleep(50 * time.Millisecond)

	server.mu.Lock()
	var sessionID string
	for id := range server.sessions {
		sessionID = id
		break
	}
	server.mu.Unlock()

	if sessionID == "" {
		t.Fatal("session was not created")
	}

	// Verify ask_question handler exists on session registry
	sess, ok := server.lookupSession(sessionID)
	if !ok {
		t.Fatalf("session %q not found", sessionID)
	}

	handler, found := sess.registry.Lookup(tool.NameAskQuestion)
	if !found {
		t.Fatalf("tool %q not found in session registry", tool.NameAskQuestion)
	}

	callArgs, _ := json.Marshal(questiontool.Request{
		Question: "Which approach?",
		Options:  []string{"Option A", "Option B"},
	})
	res, err := handler.Execute(context.Background(), tool.Call{
		ID:        "q1",
		Name:      tool.NameAskQuestion,
		Arguments: callArgs,
	})
	if err != nil {
		t.Fatalf("Execute question tool: %v", err)
	}

	if !strings.Contains(res.Output, "Option A") {
		t.Errorf("expected output to contain Option A, got %q", res.Output)
	}

	var parsed questiontool.Response
	if err := json.Unmarshal(res.StructuredOutput, &parsed); err != nil {
		t.Fatalf("unmarshal structured output: %v", err)
	}
	if parsed.Status != questiontool.StatusAnswered {
		t.Errorf("expected status answered, got %s", parsed.Status)
	}
	if parsed.Answer != "Option A" {
		t.Errorf("expected answer Option A, got %s", parsed.Answer)
	}

	cancel()
	_ = clientWriter.Close()
	_ = clientReader.Close()
	_ = serverReader.Close()
	_ = serverWriter.Close()
	<-clientDone
}
