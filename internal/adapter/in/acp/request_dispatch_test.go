package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// serveHarness drives Server.Serve over in-memory pipes and exposes decoded
// output frames one at a time, keeping input open so tests can interleave
// requests the way a real client does.
type serveHarness struct {
	t             *testing.T
	writer        *io.PipeWriter
	frames        chan map[string]json.RawMessage
	notifications chan map[string]json.RawMessage
	done          chan error
	cancel        context.CancelFunc
}

func startServeHarness(t *testing.T, server *Server) *serveHarness {
	t.Helper()
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	h := &serveHarness{
		t:             t,
		writer:        inWriter,
		frames:        make(chan map[string]json.RawMessage, 64),
		notifications: make(chan map[string]json.RawMessage, 64),
		done:          make(chan error, 1),
		cancel:        cancel,
	}
	go func() {
		err := server.Serve(ctx, inReader, outWriter)
		_ = outWriter.Close()
		h.done <- err
	}()
	go func() {
		scanner := bufio.NewScanner(outReader)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			var frame map[string]json.RawMessage
			if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
				continue
			}
			h.frames <- frame
		}
		close(h.frames)
	}()
	t.Cleanup(func() {
		cancel()
		_ = inWriter.Close()
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
			t.Error("Serve did not stop during cleanup")
		}
	})
	return h
}

func (h *serveHarness) send(line string) {
	h.t.Helper()
	if _, err := io.WriteString(h.writer, line+"\n"); err != nil {
		h.t.Fatalf("write ACP input: %v", err)
	}
}

func (h *serveHarness) request(id int, method string, params any) {
	h.t.Helper()
	encoded, err := json.Marshal(params)
	if err != nil {
		h.t.Fatal(err)
	}
	h.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, encoded))
}

func (h *serveHarness) notify(method string, params any) {
	h.t.Helper()
	encoded, err := json.Marshal(params)
	if err != nil {
		h.t.Fatal(err)
	}
	h.send(fmt.Sprintf(`{"jsonrpc":"2.0","method":%q,"params":%s}`, method, encoded))
}

// response waits for the response frame carrying id, skipping notifications and
// other responses (which are returned to the caller via the skipped list).
func (h *serveHarness) response(id int, timeout time.Duration) map[string]json.RawMessage {
	h.t.Helper()
	deadline := time.After(timeout)
	want := fmt.Sprint(id)
	for {
		select {
		case frame, ok := <-h.frames:
			if !ok {
				h.t.Fatalf("output closed while waiting for response %d", id)
			}
			if _, hasMethod := frame["method"]; hasMethod {
				select {
				case h.notifications <- frame:
				default:
				}
				continue
			}
			if string(frame["id"]) == want {
				return frame
			}
		case <-deadline:
			h.t.Fatalf("no response for request %d within %s", id, timeout)
		}
	}
}

func (h *serveHarness) expectNoResponse(id int, wait time.Duration) {
	h.t.Helper()
	deadline := time.After(wait)
	want := fmt.Sprint(id)
	for {
		select {
		case frame, ok := <-h.frames:
			if !ok {
				return
			}
			if _, hasMethod := frame["method"]; hasMethod {
				continue
			}
			if string(frame["id"]) == want {
				h.t.Fatalf("request %d responded early: %v", id, frame)
			}
		case <-deadline:
			return
		}
	}
}

func (h *serveHarness) nextFrame(timeout time.Duration) map[string]json.RawMessage {
	h.t.Helper()
	select {
	case frame, ok := <-h.frames:
		if !ok {
			h.t.Fatal("output closed")
			return nil
		}
		return frame
	case <-time.After(timeout):
		h.t.Fatalf("no frame within %s", timeout)
		return nil
	}
}

func (h *serveHarness) respond(id any, result any) {
	h.t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		h.t.Fatal(err)
	}
	h.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":%s}`, id, encoded))
}

func errorCode(t *testing.T, frame map[string]json.RawMessage) int {
	t.Helper()
	var rpcErr struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(frame["error"], &rpcErr); err != nil || len(frame["error"]) == 0 {
		t.Fatalf("frame has no error object: %v", frame)
	}
	return rpcErr.Code
}

// gatedConfigurer blocks MCP configuration until its context is cancelled or the
// gate opens, giving tests a controllable slow request.
type gatedConfigurer struct {
	started chan struct{}
	gate    chan struct{}
}

func newGatedConfigurer() *gatedConfigurer {
	return &gatedConfigurer{started: make(chan struct{}, 8), gate: make(chan struct{})}
}

func (g *gatedConfigurer) configure(ctx context.Context, _ string, _ tool.Registry, _ []MCPServerConfig) (io.Closer, error) {
	g.started <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.gate:
		return nil, nil
	}
}

func (g *gatedConfigurer) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-g.started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow request did not start")
	}
}

var slowMCP = []MCPServerConfig{{Name: "slow", Command: "mcp-server"}}

func TestCancelRequestAbortsSlowRequestWithoutBlockingReadLoop(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	gated := newGatedConfigurer()
	server.mcpRegistryConfigurer = gated.configure
	h := startServeHarness(t, server)

	h.request(7, "session/new", SessionNewParams{Cwd: t.TempDir(), MCPServers: slowMCP})
	gated.waitStarted(t)

	// The read loop stays responsive while request 7 is still running.
	h.request(8, "initialize", map[string]any{"protocolVersion": ProtocolVersion})
	if frame := h.response(8, 2*time.Second); len(frame["error"]) != 0 {
		t.Fatalf("initialize failed while a slow request was in flight: %s", frame["error"])
	}

	h.notify(methodCancelRequest, CancelRequestParams{RequestID: json.RawMessage("7")})
	frame := h.response(7, 2*time.Second)
	if got := errorCode(t, frame); got != CodeRequestCancelled {
		t.Fatalf("cancelled request error code = %d, want %d", got, CodeRequestCancelled)
	}
	assertACPSchema(t, "Error", json.RawMessage(frame["error"]))
}

func TestCancelRequestForUnknownIDIsIgnored(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	h := startServeHarness(t, server)
	h.notify(methodCancelRequest, CancelRequestParams{RequestID: json.RawMessage("999")})
	h.notify(methodCancelRequest, map[string]any{"requestId": nil})
	h.send(`{"jsonrpc":"2.0","method":"$/cancel_request","params":"garbage"}`)
	h.request(1, "initialize", map[string]any{"protocolVersion": ProtocolVersion})
	if frame := h.response(1, 2*time.Second); len(frame["error"]) != 0 {
		t.Fatalf("initialize after stray cancel failed: %s", frame["error"])
	}
}

func TestCancelRequestDistinguishesStringAndNumericIDs(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	gated := newGatedConfigurer()
	server.mcpRegistryConfigurer = gated.configure
	h := startServeHarness(t, server)

	h.send(`{"jsonrpc":"2.0","id":"7","method":"session/new","params":{"cwd":"` + t.TempDir() + `","mcpServers":[{"name":"slow","command":"mcp-server"}]}}`)
	gated.waitStarted(t)

	// Numeric 7 must not cancel string "7".
	h.notify(methodCancelRequest, CancelRequestParams{RequestID: json.RawMessage("7")})
	select {
	case frame := <-h.frames:
		t.Fatalf("string-id request was cancelled by numeric id: %v", frame)
	case <-time.After(150 * time.Millisecond):
	}
	h.notify(methodCancelRequest, CancelRequestParams{RequestID: json.RawMessage(`"7"`)})
	select {
	case frame := <-h.frames:
		if got := errorCode(t, frame); got != CodeRequestCancelled {
			t.Fatalf("error code = %d, want %d", got, CodeRequestCancelled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("string-id cancel did not abort the request")
	}
}

func TestRequestsOnOneSessionStartInArrivalOrder(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	gated := newGatedConfigurer()
	server.mcpRegistryConfigurer = gated.configure
	h := startServeHarness(t, server)

	cwd := t.TempDir()
	h.request(1, "session/resume", SessionResumeParams{SessionID: "order-1", Cwd: cwd, MCPServers: slowMCP})
	gated.waitStarted(t)
	h.request(2, "session/set_mode", SessionSetModeParams{SessionID: "order-1", ModeID: "ask"})
	h.expectNoResponse(2, 200*time.Millisecond)

	// Unordered requests still pass while the session lane is busy.
	h.request(3, "session/list", SessionListParams{})
	h.response(3, 2*time.Second)

	close(gated.gate)
	first := h.response(1, 2*time.Second)
	if len(first["error"]) != 0 {
		t.Fatalf("resume failed: %s", first["error"])
	}
	second := h.response(2, 2*time.Second)
	if len(second["error"]) != 0 {
		t.Fatalf("set_mode ordered after resume failed: %s", second["error"])
	}
}

func TestCancelRequestAbortsRequestWaitingBehindPredecessor(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	gated := newGatedConfigurer()
	server.mcpRegistryConfigurer = gated.configure
	h := startServeHarness(t, server)

	h.request(1, "session/resume", SessionResumeParams{SessionID: "queue-1", Cwd: t.TempDir(), MCPServers: slowMCP})
	gated.waitStarted(t)
	h.request(2, "session/set_mode", SessionSetModeParams{SessionID: "queue-1", ModeID: "ask"})
	h.notify(methodCancelRequest, CancelRequestParams{RequestID: json.RawMessage("2")})
	frame := h.response(2, 2*time.Second)
	if got := errorCode(t, frame); got != CodeRequestCancelled {
		t.Fatalf("queued request error code = %d, want %d", got, CodeRequestCancelled)
	}
	close(gated.gate)
	if first := h.response(1, 2*time.Second); len(first["error"]) != 0 {
		t.Fatalf("predecessor failed after queued cancel: %s", first["error"])
	}
}

func TestCancelRequestStopsInFlightPromptWithCancelledStopReason(t *testing.T) {
	runner := &blockingACPRunner{started: make(chan struct{}), canceled: make(chan struct{})}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, io.Discard)
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID
	h := startServeHarness(t, server)

	h.request(3, "session/prompt", SessionPromptParams{SessionID: sessionID, Prompt: []ContentBlock{{Type: BlockTypeText, Text: "wait"}}})
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not start")
	}
	h.notify(methodCancelRequest, CancelRequestParams{RequestID: json.RawMessage("3")})
	frame := h.response(3, 2*time.Second)
	var result SessionPromptResult
	if err := json.Unmarshal(frame["result"], &result); err != nil {
		t.Fatalf("decode prompt result: %v (%v)", err, frame)
	}
	if result.StopReason != StopReasonCancelled {
		t.Fatalf("stop reason = %q, want %q", result.StopReason, StopReasonCancelled)
	}
	assertACPSchema(t, "PromptResponse", json.RawMessage(frame["result"]))
}

func TestSessionCancelRacingSubmittedPromptIsNotLost(t *testing.T) {
	runner := &blockingACPRunner{started: make(chan struct{}), canceled: make(chan struct{})}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, io.Discard)
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID
	h := startServeHarness(t, server)

	// Prompt and cancel are written back to back; the cancel must win even if the
	// prompt goroutine has not activated the turn yet.
	h.request(4, "session/prompt", SessionPromptParams{SessionID: sessionID, Prompt: []ContentBlock{{Type: BlockTypeText, Text: "wait"}}})
	h.notify("session/cancel", SessionCancelParams{SessionID: sessionID})
	frame := h.response(4, 3*time.Second)
	var result SessionPromptResult
	if err := json.Unmarshal(frame["result"], &result); err != nil {
		t.Fatalf("decode prompt result: %v (%v)", err, frame)
	}
	if result.StopReason != StopReasonCancelled {
		t.Fatalf("stop reason = %q, want %q (frame %v)", result.StopReason, StopReasonCancelled, frame)
	}
}

func TestDuplicateInFlightRequestIDIsRejected(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	gated := newGatedConfigurer()
	server.mcpRegistryConfigurer = gated.configure
	h := startServeHarness(t, server)

	h.request(5, "session/new", SessionNewParams{Cwd: t.TempDir(), MCPServers: slowMCP})
	gated.waitStarted(t)
	h.request(5, "initialize", map[string]any{"protocolVersion": ProtocolVersion})
	select {
	case frame := <-h.frames:
		if got := errorCode(t, frame); got != CodeInvalidRequest {
			t.Fatalf("duplicate id error code = %d, want %d", got, CodeInvalidRequest)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("duplicate request id was not rejected")
	}
}
