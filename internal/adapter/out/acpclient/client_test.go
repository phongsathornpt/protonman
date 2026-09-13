package acpclient

import (
	"bytes"
	"context"
	"encoding/json"
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
