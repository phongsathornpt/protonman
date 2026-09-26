package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
)

// permissionClient is a scripted ACP client. It drives a Server over pipes and
// answers the first session/request_permission it observes with a fixed option.
type permissionClient struct {
	mu            sync.Mutex
	requests      []RequestPermissionParams
	answered      []string
	optionID      string
	requests_seen chan struct{}
	sessionID     chan string
}

func newPermissionClient(optionID string) *permissionClient {
	return &permissionClient{
		optionID:      optionID,
		requests_seen: make(chan struct{}, 8),
		sessionID:     make(chan string, 1),
	}
}

func (p *permissionClient) record(params RequestPermissionParams) {
	p.mu.Lock()
	p.requests = append(p.requests, params)
	choice := p.optionID
	p.answered = append(p.answered, choice)
	p.mu.Unlock()
	select {
	case p.requests_seen <- struct{}{}:
	default:
	}
}

func (p *permissionClient) snapshot() ([]RequestPermissionParams, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]RequestPermissionParams(nil), p.requests...), append([]string(nil), p.answered...)
}

func (p *permissionClient) awaitRequest(t *testing.T) {
	t.Helper()
	select {
	case <-p.requests_seen:
	case <-time.After(10 * time.Second):
		t.Fatal("server never sent session/request_permission")
	}
}

// runSessionWithPermissionToolCall serves one session, executes a single
// mutating tool call against that session's service in ask mode, and answers the
// resulting permission request. It returns the tool result.
func runSessionWithPermissionToolCall(t *testing.T, optionID string) (tool.Result, error) {
	t.Helper()

	// A bash-kind tool is not statically allowed, so ask mode must prompt.
	registry := acpRegistry{handler: acpHandler{definition: tool.Definition{
		Name:        "bash",
		Description: "run a shell command",
		Kind:        tool.KindBash,
	}}}
	policy, err := permission.NewPolicy(permission.Config{Default: permission.ActionAsk})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(permission.ModeAsk))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server, err := New(service, registry, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	client := newPermissionClient(optionID)

	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(context.Background(), serverReader, serverWriter) }()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		scanner := bufio.NewScanner(clientReader)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var frame struct {
				ID     *int64          `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				continue
			}
			// Capture the server-generated session ID so the tool call targets it.
			if frame.Method == "" && frame.ID != nil && *frame.ID == 2 && len(frame.Result) > 0 {
				var created struct {
					SessionID string `json:"sessionId"`
				}
				if json.Unmarshal(frame.Result, &created) == nil && created.SessionID != "" {
					select {
					case client.sessionID <- created.SessionID:
					default:
					}
				}
				continue
			}
			if frame.Method != requestPermissionMethod {
				continue
			}
			var params RequestPermissionParams
			if err := json.Unmarshal(frame.Params, &params); err != nil {
				continue
			}
			client.record(params)
			payload, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      *frame.ID,
				"result":  map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": client.optionID}},
			})
			if _, err := clientWriter.Write(append(payload, '\n')); err != nil {
				return
			}
		}
	}()

	if err := writeFrame(clientWriter, 1, "initialize", map[string]any{"protocolVersion": 1}); err != nil {
		t.Fatalf("write initialize: %v", err)
	}
	if err := writeFrame(clientWriter, 2, "session/new", map[string]any{"cwd": t.TempDir()}); err != nil {
		t.Fatalf("write session/new: %v", err)
	}
	// The session ID is server-generated, so read it from the response.
	var sessionID string
	select {
	case sessionID = <-client.sessionID:
	case <-time.After(10 * time.Second):
		t.Fatal("session/new did not return a session ID")
	}
	sess, ok := server.lookupSession(sessionID)
	if !ok {
		t.Fatalf("session %q is not registered on the server", sessionID)
	}

	// The session service must carry a prompt; without one, ask mode denies.
	callCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, callErr := sess.service.Call(callCtx, tool.Call{
		ID:        "call-1",
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"echo hi"}`),
	})

	_ = clientWriter.Close()
	_ = serverWriter.Close()
	<-readDone
	select {
	case <-serveDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the client closed")
	}
	return result, callErr
}

func writeFrame(w io.Writer, id int, method string, params any) error {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
	if err != nil {
		return err
	}
	_, err = w.Write(append(payload, '\n'))
	return err
}

// TestACPPermissionPromptAllowsOnClientApproval is the regression test for the
// bug that made Desktop unusable: ask mode denied every tool call with
// "no permission prompt is configured" because the ACP server never installed a
// prompt, so the client's permission panel could never be shown.
func TestACPPermissionPromptAllowsOnClientApproval(t *testing.T) {
	result, err := runSessionWithPermissionToolCall(t, "allow_once")
	if err != nil {
		t.Fatalf("ask-mode tool call should succeed after approval: %v", err)
	}
	if result.Output != "ok" {
		t.Fatalf("tool output = %q, want the handler to have run", result.Output)
	}
}

func TestACPPermissionPromptDeniesOnClientRejection(t *testing.T) {
	_, err := runSessionWithPermissionToolCall(t, "reject_once")
	if err == nil {
		t.Fatal("ask-mode tool call should fail when the client rejects it")
	}
	if !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "permission") {
		t.Fatalf("error = %v, want a permission denial", err)
	}
}

func TestACPPermissionRequestCarriesSessionAndToolContext(t *testing.T) {
	// Guard the payload the desktop permission panel renders.
	toolCall := permissionToolCall(permission.Request{
		CallID:   "call-9",
		ToolName: "bash",
		ToolKind: permission.ToolBash,
		Detail:   "rm -rf tmp",
	})
	if toolCall["toolCallId"] != "call-9" {
		t.Fatalf("tool call payload = %#v", toolCall)
	}
	if !strings.Contains(toolCall["title"].(string), "bash") {
		t.Fatalf("title = %v, want the tool name", toolCall["title"])
	}
	if kind := toolCall["kind"].(string); kind != string(permission.ToolBash) {
		t.Fatalf("kind = %q, want the tool kind", kind)
	}
}

func TestPermissionResolutionMapsOptions(t *testing.T) {
	once, err := permissionResolution("allow_once")
	if err != nil || once.Action != permission.ActionAllow || once.Scope != permission.GrantScopeOnce {
		t.Fatalf("allow_once = %#v, err %v", once, err)
	}
	always, err := permissionResolution("allow_session")
	if err != nil || always.Action != permission.ActionAllow || always.Scope != permission.GrantScopeSession {
		t.Fatalf("allow_session = %#v, err %v", always, err)
	}
	for _, optionID := range []string{"reject_once", "reject_always", "", "garbage"} {
		got, err := permissionResolution(optionID)
		if err != nil || got.Action != permission.ActionDeny {
			t.Fatalf("permissionResolution(%q) = %#v, err %v; want deny", optionID, got, err)
		}
	}
}

func TestPermissionBrokerCloseReleasesWaiters(t *testing.T) {
	server := &Server{output: io.Discard}
	broker := newPermissionBroker(server)
	if _, _, err := broker.register(); err != nil {
		t.Fatalf("register: %v", err)
	}
	broker.close()
	if _, _, err := broker.register(); err == nil {
		t.Fatal("a closed broker must refuse new permission requests")
	}
}
