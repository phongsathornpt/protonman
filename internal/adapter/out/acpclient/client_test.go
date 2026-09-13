package acpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

type bufferWriteCloser struct{ bytes.Buffer }

func (b *bufferWriteCloser) Close() error { return nil }

func newTestClient(t *testing.T) (*Client, *bufferWriteCloser) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	writer := &bufferWriteCloser{}
	return &Client{
		ctx: ctx,
		stdin: writer,
		cancel: cancel,
		pending: make(map[uint64]chan response),
		closed: make(chan struct{}),
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

func TestReadLoopSeparatesNotificationsAndReverseRequests(t *testing.T) {
	client, writer := newTestClient(t)
	events := make(chan Event, 1)
	requests := make(chan Request, 1)
	client.onEvent = func(event Event) { events <- event }
	client.SetRequestHandler(func(_ context.Context, request Request) (any, error) {
		requests <- request
		return map[string]any{"ok": true}, nil
	})

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1"}}`,
		`{"jsonrpc":"2.0","id":11,"method":"session/request_permission","params":{"sessionId":"s1"}}`,
	}, "\n") + "\n"
	client.readLoop(strings.NewReader(input))

	select {
	case event := <-events:
		if event.Method != "session/update" {
			t.Fatalf("event method = %q", event.Method)
		}
	default:
		t.Fatal("notification was not delivered")
	}
	select {
	case request := <-requests:
		if request.Method != "session/request_permission" {
			t.Fatalf("request method = %q", request.Method)
		}
	default:
		t.Fatal("reverse request was not delivered")
	}
	if writer.Len() == 0 {
		t.Fatal("reverse request response was not written")
	}
}

var _ io.WriteCloser = (*bufferWriteCloser)(nil)
