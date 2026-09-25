package acpclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

type bufferWriteCloser struct{ bytes.Buffer }

func (b *bufferWriteCloser) Close() error { return nil }

func newTestClient(t *testing.T) (*Client, *bufferWriteCloser) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	writer := &bufferWriteCloser{}
	return &Client{
		ctx:     ctx,
		stdin:   writer,
		cancel:  cancel,
		pending: make(map[uint64]chan response),
		closed:  make(chan struct{}),
	}, writer
}

func TestHandleRequestWritesResult(t *testing.T) {
	client, writer := newTestClient(t)
	client.SetRequestHandler(func(_ context.Context, request Request) (any, error) {
		if request.Method != "session/request_permission" {
			t.Fatalf("method = %q", request.Method)
		}
		return map[string]any{"outcome": "ok"}, nil
	})

	client.handleRequest(Request{ID: json.RawMessage("7"), Method: "session/request_permission", Params: json.RawMessage(`{"sessionId":"s1"}`)})

	var got envelope
	if err := json.Unmarshal(bytes.TrimSpace(writer.Bytes()), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if string(got.ID) != "7" || got.Error != nil {
		t.Fatalf("response = %#v", got)
	}
	if !strings.Contains(string(got.Result), `"outcome":"ok"`) {
		t.Fatalf("result = %s", got.Result)
	}
}

func TestHandleRequestWithoutHandlerWritesMethodNotFound(t *testing.T) {
	client, writer := newTestClient(t)
	client.handleRequest(Request{ID: json.RawMessage("9"), Method: "unknown"})

	var got envelope
	if err := json.Unmarshal(bytes.TrimSpace(writer.Bytes()), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error == nil || got.Error.Code != -32601 {
		t.Fatalf("error = %#v", got.Error)
	}
}

func TestCommandSpecIsIndependentFromLegacyStart(t *testing.T) {
	// Keep this contract test close to the transport: integrations must be able
	// to provide an ACP executable that does not accept Protonman's --acp flag.
	spec := CommandSpec{Path: "/opt/agy/agy_acp_server.par", Args: []string{"--uid=desktop"}}
	if len(spec.Args) != 1 || spec.Args[0] == "--acp" {
		t.Fatalf("command spec = %#v", spec)
	}
}

// TestHelperACPProcess acts as a scripted ACP agent driven by ACP_HELPER_MODE.
func TestHelperACPProcess(t *testing.T) {
	mode := os.Getenv("ACP_HELPER_MODE")
	switch mode {
	case "stderr_then_exit":
		// Bootstrap failures all land on stderr; the desktop must surface them
		// instead of reporting a bare handshake timeout. Wait for a request first
		// so the failure is observed by a call that is already in flight.
		fmt.Fprint(os.Stderr, "create ACP server: config load failed")
		bufio.NewReader(os.Stdin).ReadString('\n')
		os.Exit(1)
	case "rpc_error":
		reader := bufio.NewReader(os.Stdin)
		if _, err := reader.ReadString('\n'); err != nil {
			return
		}
		fmt.Fprint(os.Stdout, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"session/list is not supported"}}`+"\n")
		// Stay alive so the read loop is the one that observes the response.
		time.Sleep(2 * time.Second)
	case "noisy_then_error":
		// A banner on stdout must not break the framed protocol reader.
		fmt.Fprint(os.Stdout, "agent starting up\n")
		reader := bufio.NewReader(os.Stdin)
		if _, err := reader.ReadString('\n'); err != nil {
			return
		}
		fmt.Fprint(os.Stdout, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"upstream refused"}}`+"\n")
		time.Sleep(2 * time.Second)
	}
}

func startHelperClient(t *testing.T, mode string) *Client {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	client, err := StartCommand(context.Background(), CommandSpec{
		Path: executable,
		Args: []string{"-test.run=TestHelperACPProcess"},
		Env:  []string{"ACP_HELPER_MODE=" + mode},
	}, nil)
	if err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func waitForDone(t *testing.T, client *Client) {
	t.Helper()
	select {
	case <-client.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("client did not shut down")
	}
}

func TestStderrTailIsReportedOnAbnormalExit(t *testing.T) {
	client := startHelperClient(t, "stderr_then_exit")

	// The call is issued first so it is pending when the child dies; the
	// annotated cause must reach the caller rather than a bare closed error.
	type outcome struct{ err error }
	result := make(chan outcome, 1)
	go func() {
		var payload map[string]any
		result <- outcome{err: client.Call(context.Background(), "initialize", map[string]any{}, &payload)}
	}()

	select {
	case got := <-result:
		if got.err == nil || !strings.Contains(got.err.Error(), "config load failed") {
			t.Fatalf("call error = %v, want the child's stderr to explain the failure", got.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("call did not return after the child exited")
	}

	if tail := client.stderrTail(); !strings.Contains(tail, "create ACP server: config load failed") {
		t.Fatalf("stderr tail = %q, want the child's diagnostics", tail)
	}
}

func TestJSONRPCErrorResponsePropagates(t *testing.T) {
	client := startHelperClient(t, "rpc_error")

	var result map[string]any
	err := client.Call(context.Background(), "session/list", map[string]any{}, &result)

	var rpcError *RPCError
	if !errors.As(err, &rpcError) {
		t.Fatalf("error = %v, want *RPCError", err)
	}
	if rpcError.Code != -32601 || !strings.Contains(rpcError.Message, "session/list is not supported") {
		t.Fatalf("rpc error = %#v", rpcError)
	}
}

func TestMalformedStdoutLineDoesNotBreakFraming(t *testing.T) {
	client := startHelperClient(t, "noisy_then_error")

	var result map[string]any
	err := client.Call(context.Background(), "session/list", map[string]any{}, &result)

	var rpcError *RPCError
	if !errors.As(err, &rpcError) || rpcError.Code != -32000 {
		t.Fatalf("error = %v, want the framed response after the banner line", err)
	}
}

func TestStderrTailIsBounded(t *testing.T) {
	client := &Client{}
	oversized := bytes.Repeat([]byte("x"), stderrTailLimit*3)
	client.appendStderr(oversized)

	tail := client.stderrTail()
	if len(tail) != stderrTailLimit {
		t.Fatalf("tail length = %d, want it bounded to %d", len(tail), stderrTailLimit)
	}
}

func TestWithStderrLeavesCleanCauseUntouched(t *testing.T) {
	client := &Client{}
	cause := errors.New("exit status 1")

	got := client.withStderr(cause)
	if got != cause {
		t.Fatalf("withStderr = %v, want the original cause when stderr is empty", got)
	}
}

func TestStderrDrainTerminatesOnReaderEOF(t *testing.T) {
	client := &Client{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		client.drainStderr(strings.NewReader("partial output"))
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drainStderr did not return at EOF")
	}
	if tail := client.stderrTail(); tail != "partial output" {
		t.Fatalf("stderr tail = %q", tail)
	}
}
