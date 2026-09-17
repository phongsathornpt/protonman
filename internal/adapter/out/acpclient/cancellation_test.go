package acpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

type signalingWriteCloser struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	first chan struct{}
	once  sync.Once
}

func (w *signalingWriteCloser) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	w.once.Do(func() { close(w.first) })
	return n, err
}

func (w *signalingWriteCloser) Close() error { return nil }

func (w *signalingWriteCloser) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestCallCancellationNotifiesAgent(t *testing.T) {
	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()
	writer := &signalingWriteCloser{first: make(chan struct{})}
	client := &Client{
		ctx:     clientCtx,
		stdin:   writer,
		cancel:  clientCancel,
		pending: make(map[uint64]chan response),
		closed:  make(chan struct{}),
	}

	callCtx, cancelCall := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.Call(callCtx, "session/prompt", map[string]any{"sessionId": "s1"}, nil)
	}()

	<-writer.first
	cancelCall()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Call() error = %v, want context.Canceled", err)
	}

	lines := strings.Split(strings.TrimSpace(writer.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("writes = %d, want request plus cancellation: %s", len(lines), writer.String())
	}
	var cancel envelope
	if err := json.Unmarshal([]byte(lines[1]), &cancel); err != nil {
		t.Fatalf("decode cancellation notification: %v", err)
	}
	if cancel.Method != cancelRequestMethod || len(cancel.ID) != 0 {
		t.Fatalf("cancellation envelope = %#v", cancel)
	}
	var params struct {
		RequestID uint64 `json:"requestId"`
	}
	if err := json.Unmarshal(cancel.Params, &params); err != nil {
		t.Fatalf("decode cancellation params: %v", err)
	}
	if params.RequestID != 1 {
		t.Fatalf("requestId = %d, want 1", params.RequestID)
	}
	client.mu.Lock()
	pending := len(client.pending)
	client.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending requests = %d, want 0 after cancellation", pending)
	}
}
