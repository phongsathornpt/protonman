package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/adapter/out/sessionfs"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestACPInitializeAndPrompt(t *testing.T) {
	server := newTestServer(t, permission.ModeAlwaysApprove)
	var output bytes.Buffer
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/test/dir"}}`,
	}, "\n") + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	outStr := output.String()
	if !strings.Contains(outStr, `"protocolVersion":1`) {
		t.Fatalf("initialize missing version: %s", outStr)
	}
	if !strings.Contains(outStr, `"loadSession":true`) {
		t.Fatalf("initialize missing loadSession: true: %s", outStr)
	}
	if !strings.Contains(outStr, `"available_commands_update"`) {
		t.Fatalf("session/new missing available_commands_update: %s", outStr)
	}
	if !strings.Contains(outStr, `"name":"reasoning"`) {
		t.Fatalf("session/new missing reasoning command advertisement: %s", outStr)
	}
	if !strings.Contains(outStr, `"sessionId"`) {
		t.Fatalf("session/new missing sessionId: %s", outStr)
	}

	sessionID := extractSessionID(t, output.Bytes())
	output.Reset()
	prompt := `{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sessionID + `","prompt":[{"type":"text","text":"/tools"}]}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(prompt), &output); err != nil {
		t.Fatalf("prompt Serve() error = %v", err)
	}
	if !strings.Contains(output.String(), "session/update") {
		t.Fatalf("missing session/update: %s", output.String())
	}
	if !strings.Contains(output.String(), "read_file") {
		t.Fatalf("prompt output missing tools: %s", output.String())
	}
	if !strings.Contains(output.String(), `"stopReason":"end_turn"`) {
		t.Fatalf("missing end_turn: %s", output.String())
	}
}

func TestACPDirectCall(t *testing.T) {
	server := newTestServer(t, permission.ModeAlwaysApprove)
	var output bytes.Buffer
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/test/dir"}}`,
	}, "\n") + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	sessionID := extractSessionID(t, output.Bytes())
	output.Reset()

	prompt := `{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sessionID + `","prompt":[{"type":"text","text":"/call read_file {\"path\":\"test.txt\"}"}]}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(prompt), &output); err != nil {
		t.Fatalf("prompt Serve() error = %v", err)
	}
	out := output.String()
	if !strings.Contains(out, `"sessionUpdate":"tool_call"`) {
		t.Fatalf("missing tool_call update: %s", out)
	}
	if !strings.Contains(out, `"sessionUpdate":"tool_call_update"`) {
		t.Fatalf("missing tool_call_update: %s", out)
	}
	if !strings.Contains(out, `"status":"completed"`) {
		t.Fatalf("missing status completed: %s", out)
	}
	if !strings.Contains(out, `"stopReason":"end_turn"`) {
		t.Fatalf("missing stopReason end_turn: %s", out)
	}
}

func TestACPStreamingAndToolCalls(t *testing.T) {
	runner := &streamingACPRunner{}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)

	var output bytes.Buffer
	input := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve session/new error = %v", err)
	}
	sessionID := extractSessionID(t, output.Bytes())
	output.Reset()

	prompt := `{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"` + sessionID + `","prompt":[{"type":"text","text":"hello"}]}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(prompt), &output); err != nil {
		t.Fatalf("Serve prompt error = %v", err)
	}

	out := output.String()
	if !strings.Contains(out, `"sessionUpdate":"agent_message_chunk"`) {
		t.Fatalf("missing agent_message_chunk: %s", out)
	}
	if !strings.Contains(out, `"sessionUpdate":"tool_call"`) {
		t.Fatalf("missing tool_call update: %s", out)
	}
	if !strings.Contains(out, `"sessionUpdate":"tool_call_update"`) {
		t.Fatalf("missing tool_call_update: %s", out)
	}
	if !strings.Contains(out, `"locations":[{"path":"test.go"}]`) {
		t.Fatalf("missing tool call locations for Follow-the-Agent: %s", out)
	}
	if !strings.Contains(out, `"stopReason":"end_turn"`) {
		t.Fatalf("missing end_turn: %s", out)
	}
}

func TestACPSessionModes(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	var output bytes.Buffer

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{}}`,
	}, "\n") + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve error = %v", err)
	}
	sessionID := extractSessionID(t, output.Bytes())
	output.Reset()

	// Switch mode via set_mode
	setMode := `{"jsonrpc":"2.0","id":2,"method":"session/set_mode","params":{"sessionId":"` + sessionID + `","modeId":"plan"}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(setMode), &output); err != nil {
		t.Fatalf("Serve set_mode error = %v", err)
	}
	if !strings.Contains(output.String(), `"current_mode_update"`) {
		t.Fatalf("missing current_mode_update: %s", output.String())
	}
	if !strings.Contains(output.String(), `"modeId":"deny"`) && !strings.Contains(output.String(), `"modeId":"plan"`) {
		t.Fatalf("set_mode missing plan/deny mode update: %s", output.String())
	}
}

func TestACPSessionsHaveIndependentPermissionState(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	var output bytes.Buffer
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{}}`,
	}, "\n") + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	if len(server.sessions) != 2 {
		t.Fatalf("sessions = %#v, want two sessions", server.sessions)
	}
	var first, second *Session
	for _, sess := range server.sessions {
		if first == nil {
			first = sess
		} else {
			second = sess
		}
	}
	if first.service == second.service {
		t.Fatal("sessions share the same tool-call service")
	}
	if err := first.service.SetMode(permission.ModeDeny); err != nil {
		t.Fatalf("SetMode() error = %v", err)
	}
	if got := second.service.Mode(); got != permission.ModeAsk {
		t.Fatalf("second session mode = %s, want ask", got)
	}
}

func TestACPSessionListAndDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "acp-store-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := sessionfs.NewFileStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	server := newTestServer(t, permission.ModeAsk)
	server.sessionService = app.NewSessions(store)

	var output bytes.Buffer
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/test/project"}}`,
	}, "\n") + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve error = %v", err)
	}
	sessionID := extractSessionID(t, output.Bytes())
	output.Reset()

	// List sessions
	listReq := `{"jsonrpc":"2.0","id":2,"method":"session/list","params":{}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(listReq), &output); err != nil {
		t.Fatalf("Serve list error = %v", err)
	}
	if !strings.Contains(output.String(), sessionID) {
		t.Fatalf("session/list missing active session: %s", output.String())
	}
	output.Reset()

	// Delete session
	delReq := `{"jsonrpc":"2.0","id":3,"method":"session/delete","params":{"sessionId":"` + sessionID + `"}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(delReq), &output); err != nil {
		t.Fatalf("Serve delete error = %v", err)
	}
	output.Reset()

	// List again - should be empty
	if err := server.Serve(context.Background(), strings.NewReader(listReq), &output); err != nil {
		t.Fatalf("Serve list error = %v", err)
	}
	if strings.Contains(output.String(), sessionID) {
		t.Fatalf("session/list should not contain deleted session: %s", output.String())
	}
}

func TestACPSessionLoadAndReplay(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "acp-replay-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := sessionfs.NewFileStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	sessionID := "test-session-1"
	err = store.Save(context.Background(), sessionID, session.State{
		PermissionMode: "ask",
		Messages: []session.Message{
			{Role: model.RoleUser, Content: "Hello Protonman"},
			{Role: model.RoleAssistant, Content: "Hello from Protonman Agent"},
		},
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	server := newTestServer(t, permission.ModeAsk)
	server.sessionService = app.NewSessions(store)

	var output bytes.Buffer
	loadReq := `{"jsonrpc":"2.0","id":1,"method":"session/load","params":{"sessionId":"` + sessionID + `"}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(loadReq), &output); err != nil {
		t.Fatalf("Serve load error = %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "user_message_chunk") || !strings.Contains(out, "Hello Protonman") {
		t.Fatalf("missing replayed user message: %s", out)
	}
	if !strings.Contains(out, "agent_message_chunk") || !strings.Contains(out, "Hello from Protonman Agent") {
		t.Fatalf("missing replayed assistant message: %s", out)
	}
}

func TestACPPromptSurfacesPersistenceFailure(t *testing.T) {
	store := invalidSessionStore(t)
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, &streamingACPRunner{})
	server.sessionService = app.NewSessions(store)
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID
	sess, ok := server.lookupSession(sessionID)
	if !ok {
		t.Fatalf("session %q not found", sessionID)
	}

	_, err = sess.ExecutePrompt(context.Background(), []ContentBlock{{Type: BlockTypeText, Text: "read"}}, func(RPCNotification) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "save session") {
		t.Fatalf("ExecutePrompt() error = %v, want persistence error", err)
	}
}

func TestACPStorageOperationFailuresSurface(t *testing.T) {
	store := invalidSessionStore(t)
	server := newTestServer(t, permission.ModeAsk)
	server.sessionService = app.NewSessions(store)
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID

	_, _, err = server.dispatch(context.Background(), RPCRequest{Method: "session/list"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "list session state") {
		t.Fatalf("session/list error = %v, want storage error", err)
	}
	_, _, err = server.dispatch(context.Background(), RPCRequest{
		Method: "session/delete",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `"}`),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "delete session state") {
		t.Fatalf("session/delete error = %v, want storage error", err)
	}
	if _, ok := server.lookupSession(sessionID); !ok {
		t.Fatal("session was removed after persistent delete failure")
	}
}

func TestACPCancelStopsInFlightPrompt(t *testing.T) {
	runner := &blockingACPRunner{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)
	var output bytes.Buffer
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, &output)
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID

	reader, writer := io.Pipe()
	output.Reset()
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(context.Background(), reader, &output)
	}()

	prompt := `{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sessionID + `","prompt":[{"type":"text","text":"wait"}]}}` + "\n"
	if _, err := io.WriteString(writer, prompt); err != nil {
		t.Fatalf("write prompt error = %v", err)
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		_ = writer.Close()
		t.Fatal("runner did not start prompt")
	}

	cancel := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"` + sessionID + `"}}` + "\n"
	if _, err := io.WriteString(writer, cancel); err != nil {
		t.Fatalf("write cancel error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close ACP input error = %v", err)
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not finish after cancellation")
	}
	select {
	case <-runner.canceled:
	default:
		t.Fatal("runner did not observe prompt cancellation")
	}
	if !strings.Contains(output.String(), `"stopReason":"cancelled"`) {
		t.Fatalf("cancelled prompt response = %s", output.String())
	}
}

func TestACPCancelEmitsTerminalToolUpdate(t *testing.T) {
	runner := &cancelAfterToolCallRunner{started: make(chan struct{})}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID
	sess, ok := server.lookupSession(sessionID)
	if !ok {
		t.Fatalf("session %q not found", sessionID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var notifications []RPCNotification
	resultCh := make(chan error, 1)
	go func() {
		_, runErr := sess.ExecutePrompt(ctx, []ContentBlock{{Type: BlockTypeText, Text: "wait"}}, func(notification RPCNotification) error {
			notifications = append(notifications, notification)
			return nil
		})
		resultCh <- runErr
	}()

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start prompt")
	}
	cancel()
	select {
	case runErr := <-resultCh:
		if runErr != nil {
			t.Fatalf("ExecutePrompt() error = %v, want nil cancellation result", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ExecutePrompt() did not finish after cancellation")
	}

	raw, err := json.Marshal(notifications)
	if err != nil {
		t.Fatalf("marshal notifications: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"toolCallId":"call-cancel"`) || !strings.Contains(text, `"status":"failed"`) {
		t.Fatalf("notifications missing terminal failed tool update: %s", text)
	}
	if messages := sess.Messages(); len(messages) != 0 {
		t.Fatalf("canceled prompt persisted partial transcript: %+v", messages)
	}
}

func TestACPFailedPromptRollsBackPartialToolTranscript(t *testing.T) {
	runner := &failingAfterToolCallRunner{}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID
	sess, ok := server.lookupSession(sessionID)
	if !ok {
		t.Fatalf("session %q not found", sessionID)
	}

	_, err = sess.ExecutePrompt(context.Background(), []ContentBlock{{Type: BlockTypeText, Text: "fail"}}, func(RPCNotification) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "runner failed") {
		t.Fatalf("ExecutePrompt() error = %v, want runner failure", err)
	}
	if messages := sess.Messages(); len(messages) != 0 {
		t.Fatalf("failed prompt persisted partial transcript: %+v", messages)
	}
}

func TestACPUnknownMethod(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	var output bytes.Buffer
	err := server.Serve(
		context.Background(),
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"nope","params":{}}`+"\n"),
		&output,
	)
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if !strings.Contains(output.String(), "not supported") {
		t.Fatalf("unknown method response = %s", output.String())
	}
}

func FuzzACPRequest(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":2,"method":"session/new"}`))
	f.Add([]byte(`not-json`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, line []byte) {
		server := newTestServer(t, permission.ModeAsk)
		var output bytes.Buffer
		_ = server.Serve(context.Background(), bytes.NewReader(append(append([]byte{}, line...), '\n')), &output)
	})
}

func extractSessionID(t *testing.T, raw []byte) string {
	t.Helper()
	for line := range bytes.SplitSeq(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var payload struct {
			Result struct {
				SessionID string `json:"sessionId"`
			} `json:"result"`
		}
		if err := json.Unmarshal(line, &payload); err != nil {
			continue
		}
		if payload.Result.SessionID != "" {
			return payload.Result.SessionID
		}
	}
	t.Fatalf("session id not found in %s", raw)
	return ""
}

func invalidSessionStore(t *testing.T) *sessionfs.FileStore {
	t.Helper()
	root := t.TempDir() + "/not-a-directory"
	if err := os.WriteFile(root, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("create invalid store root: %v", err)
	}
	store, err := sessionfs.NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	return store
}

type acpHandler struct {
	definition tool.Definition
}

func (h acpHandler) Definition() tool.Definition { return h.definition }

func (h acpHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: "ok"}, nil
}

type acpRegistry struct {
	handler tool.Handler
}

func (r acpRegistry) Lookup(name string) (tool.Handler, bool) {
	if name != r.handler.Definition().Name {
		return nil, false
	}
	return r.handler, true
}

func (r acpRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.Definition()}
}

type blockingACPRunner struct {
	started  chan struct{}
	canceled chan struct{}
}

type cancelAfterToolCallRunner struct {
	started chan struct{}
}

type failingAfterToolCallRunner struct{}

func (r *failingAfterToolCallRunner) Run(ctx context.Context, _ []model.Message, sink applicationturn.Sink) (applicationturn.Result, error) {
	call := tool.Call{
		ID:        "call-fail",
		Name:      "read_file",
		Arguments: json.RawMessage(`{"path":"test.go"}`),
	}
	if err := sink(ctx, applicationturn.Event{Kind: applicationturn.EventToolCall, Call: call}); err != nil {
		return applicationturn.Result{}, err
	}
	return applicationturn.Result{
		Messages: []model.Message{{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{{
				ID: call.ID, Name: call.Name, Arguments: call.Arguments,
			}},
		}},
	}, errors.New("runner failed")
}

func (r *cancelAfterToolCallRunner) Run(ctx context.Context, _ []model.Message, sink applicationturn.Sink) (applicationturn.Result, error) {
	call := tool.Call{
		ID:        "call-cancel",
		Name:      "read_file",
		Arguments: json.RawMessage(`{"path":"test.go"}`),
	}
	if err := sink(ctx, applicationturn.Event{Kind: applicationturn.EventToolCall, Call: call}); err != nil {
		return applicationturn.Result{}, err
	}
	close(r.started)
	<-ctx.Done()
	_ = sink(context.WithoutCancel(ctx), applicationturn.Event{
		Kind: applicationturn.EventFailed,
		Err:  ctx.Err(),
	})
	return applicationturn.Result{}, ctx.Err()
}

func (r *blockingACPRunner) Run(ctx context.Context, _ []model.Message, _ applicationturn.Sink) (applicationturn.Result, error) {
	close(r.started)
	<-ctx.Done()
	close(r.canceled)
	return applicationturn.Result{}, ctx.Err()
}

type streamingACPRunner struct{}

func (r *streamingACPRunner) Run(ctx context.Context, _ []model.Message, sink applicationturn.Sink) (applicationturn.Result, error) {
	// Emit streaming text delta
	_ = sink(ctx, applicationturn.Event{
		Kind: applicationturn.EventTextDelta,
		Text: "Thinking...",
	})

	call := tool.Call{
		ID:        "call-123",
		Name:      "read_file",
		Arguments: json.RawMessage(`{"path":"test.go"}`),
	}

	// Emit tool call
	_ = sink(ctx, applicationturn.Event{
		Kind: applicationturn.EventToolCall,
		Call: call,
	})

	// Emit tool result
	_ = sink(ctx, applicationturn.Event{
		Kind: applicationturn.EventToolResult,
		Call: call,
		Result: tool.Result{
			CallID:   "call-123",
			ToolName: "read_file",
			Output:   "package main",
		},
	})

	// Emit completion
	_ = sink(ctx, applicationturn.Event{
		Kind: applicationturn.EventCompleted,
		Message: model.Message{
			Role:    model.RoleAssistant,
			Content: "Done reading file",
		},
	})

	return applicationturn.Result{
		Message: model.Message{
			Role:    model.RoleAssistant,
			Content: "Done reading file",
		},
	}, nil
}

func newTestServer(t *testing.T, mode permission.Mode) *Server {
	t.Helper()
	return newTestServerWithRunner(t, mode, nil)
}

func newTestServerWithRunner(t *testing.T, mode permission.Mode, runner applicationturn.Runner) *Server {
	t.Helper()
	registry := acpRegistry{handler: acpHandler{definition: tool.Definition{
		Name:        "read_file",
		Description: "read a file",
		Kind:        tool.KindRead,
	}}}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(mode))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	options := []Option(nil)
	if runner != nil {
		options = append(options, WithRunnerFactory(func(*toolcall.Service) (app.Conversation, error) {
			return runner, nil
		}))
	}
	server, err := New(service, registry, runner, options...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return server
}

func TestServeCancellationInterruptsClosableInput(t *testing.T) {
	server := newTestServer(t, permission.ModeAlwaysApprove)
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, reader, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
}

func TestACPReasoningSlashPersistsAndValidatesModelProfile(t *testing.T) {
	loop := newACPReasoningLoop(t, "gemini-3.8-flash")
	server := newTestServerWithRunner(t, permission.ModeAsk, loop)
	store, err := sessionfs.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server.sessionService = app.NewSessions(store)
	sess, err := server.newSession(context.Background(), "reasoning-session", "/tmp", nil)
	if err != nil {
		t.Fatal(err)
	}
	notify := func(RPCNotification) error { return nil }
	if handled, _, err := sess.handleSlashCommand(context.Background(), model.Message{}, "/reasoning high", notify); !handled || err != nil {
		t.Fatalf("/reasoning high = handled %v, err %v", handled, err)
	}
	if got := sess.ReasoningEffort(); got != sdk.ReasoningHigh {
		t.Fatalf("reasoning effort = %q, want high", got)
	}
	loaded, found, err := store.Load(context.Background(), "reasoning-session")
	if err != nil || !found || loaded.ReasoningEffort != "high" {
		t.Fatalf("persisted reasoning = %#v, found %v, err %v", loaded.ReasoningEffort, found, err)
	}
	if handled, _, err := sess.handleSlashCommand(context.Background(), model.Message{}, "/reasoning xhigh", notify); !handled || err == nil {
		t.Fatalf("/reasoning xhigh = handled %v, err %v", handled, err)
	}
	if got := sess.ReasoningEffort(); got != sdk.ReasoningHigh {
		t.Fatalf("unsupported override changed effort to %q", got)
	}
	if handled, _, err := sess.handleSlashCommand(context.Background(), model.Message{}, "/reasoning auto", notify); !handled || err != nil {
		t.Fatalf("/reasoning auto = handled %v, err %v", handled, err)
	}
	if got := sess.ReasoningEffort(); got != sdk.ReasoningDefault {
		t.Fatalf("reasoning effort = %q, want auto", got)
	}
}

func TestACPSessionLoadRestoresReasoningEffort(t *testing.T) {
	loop := newACPReasoningLoop(t, "gemini-3.8-flash")
	server := newTestServerWithRunner(t, permission.ModeAsk, loop)
	store, err := sessionfs.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server.sessionService = app.NewSessions(store)
	if err := store.Save(context.Background(), "resume-reasoning", session.State{PermissionMode: "ask", ReasoningEffort: "high"}); err != nil {
		t.Fatal(err)
	}
	sess, err := server.loadOrCreateSession(context.Background(), "resume-reasoning", "/tmp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.ReasoningEffort(); got != sdk.ReasoningHigh {
		t.Fatalf("restored reasoning = %q, want high", got)
	}
}

func newACPReasoningLoop(t *testing.T, modelID string) *applicationturn.Loop {
	t.Helper()
	registry := acpRegistry{handler: acpHandler{definition: tool.Definition{Name: "read_file", Description: "read", Kind: tool.KindRead}}}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(permission.ModeAsk))
	if err != nil {
		t.Fatal(err)
	}
	languageModel := model.NewProviderLanguageModel(model.DefaultProtonmanName, string(model.ProviderProtocolOpenAI), "http://127.0.0.1", "", modelID)
	loop, err := applicationturn.NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	return loop
}

func TestACPMCPServerConfigsReachSessionConfigurer(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	var configured [][]MCPServerConfig
	server.mcpRegistryConfigurer = func(_ context.Context, _ string, _ tool.Registry, configs []MCPServerConfig) (io.Closer, error) {
		configured = append(configured, cloneMCPServerConfigs(configs))
		return nil, nil
	}
	configs := []MCPServerConfig{{Name: "local", Command: "mcp-server", Args: []string{"--stdio"}}}
	requests := []RPCRequest{
		{Method: "session/new", Params: mustJSON(t, SessionNewParams{Cwd: "/tmp/a", MCPServers: configs})},
		{Method: "session/load", Params: mustJSON(t, SessionLoadParams{SessionID: "load-mcp", Cwd: "/tmp/b", MCPServers: configs})},
		{Method: "session/resume", Params: mustJSON(t, SessionResumeParams{SessionID: "resume-mcp", Cwd: "/tmp/c", MCPServers: configs})},
	}
	for _, request := range requests {
		if _, _, err := server.dispatch(context.Background(), request, io.Discard); err != nil {
			t.Fatalf("%s error = %v", request.Method, err)
		}
	}
	if len(configured) != len(requests) {
		t.Fatalf("configured calls = %d, want %d", len(configured), len(requests))
	}
	for _, got := range configured {
		if !sameMCPServerConfigs(got, configs) {
			t.Fatalf("configured servers = %#v, want %#v", got, configs)
		}
	}
}

func TestACPMCPServerConfigRejectsInvalidAndUnavailable(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	invalid := []MCPServerConfig{{Name: "bad", Command: ""}}
	if _, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new", Params: mustJSON(t, SessionNewParams{MCPServers: invalid})}, io.Discard); err == nil {
		t.Fatal("invalid MCP config error = nil")
	}
	valid := []MCPServerConfig{{Name: "local", Command: "mcp-server"}}
	if _, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new", Params: mustJSON(t, SessionNewParams{MCPServers: valid})}, io.Discard); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("missing MCP configurer error = %v", err)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
