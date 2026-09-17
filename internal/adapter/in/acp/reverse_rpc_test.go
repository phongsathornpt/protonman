package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type synchronizedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *synchronizedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func TestReverseRPCRequestCorrelatesClientResponse(t *testing.T) {
	server := &Server{}
	output := &synchronizedBuffer{}
	unbind := server.bindReverseRPCOutput(output)
	defer unbind()

	type result struct {
		Outcome string `json:"outcome"`
	}
	var got result
	done := make(chan error, 1)
	go func() {
		done <- server.requestClient(context.Background(), "example/request", map[string]any{"value": 1}, &got)
	}()

	var request RPCRequest
	deadline := time.Now().Add(time.Second)
	for {
		line := strings.TrimSpace(output.String())
		if line != "" {
			if err := json.Unmarshal([]byte(line), &request); err != nil {
				t.Fatalf("decode reverse request: %v", err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reverse request was not written")
		}
		time.Sleep(time.Millisecond)
	}
	if request.Method != "example/request" || len(request.ID) == 0 {
		t.Fatalf("reverse request = %#v", request)
	}

	response := []byte(`{"jsonrpc":"2.0","id":` + string(request.ID) + `,"result":{"outcome":"selected"}}`)
	if handled := server.handleClientResponseLine(response); !handled {
		t.Fatal("client response was not handled")
	}
	if err := <-done; err != nil {
		t.Fatalf("requestClient() error = %v", err)
	}
	if got.Outcome != "selected" {
		t.Fatalf("outcome = %q, want selected", got.Outcome)
	}
}

func TestReverseRPCCancellationNotifiesClient(t *testing.T) {
	server := &Server{}
	output := &synchronizedBuffer{}
	unbind := server.bindReverseRPCOutput(output)
	defer unbind()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.requestClient(ctx, "example/request", map[string]any{}, nil)
	}()

	deadline := time.Now().Add(time.Second)
	for output.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if output.Len() == 0 {
		t.Fatal("reverse request was not written")
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("requestClient() error = %v, want context.Canceled", err)
	}

	text := output.String()
	scanner := bufio.NewScanner(strings.NewReader(text))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != 2 {
		t.Fatalf("writes = %d, want request plus cancellation: %s", len(lines), text)
	}
	var notification RPCNotification
	if err := json.Unmarshal([]byte(lines[1]), &notification); err != nil {
		t.Fatalf("decode cancellation: %v", err)
	}
	if notification.Method != methodCancelRequest {
		t.Fatalf("cancellation method = %q, want %q", notification.Method, methodCancelRequest)
	}
}

func TestHandleClientResponseLineRejectsRequests(t *testing.T) {
	server := &Server{}
	if handled := server.handleClientResponseLine([]byte(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{}}`)); handled {
		t.Fatal("JSON-RPC request was misclassified as a response")
	}
	if handled := server.handleClientResponseLine([]byte(`{"jsonrpc":"2.0","id":1}`)); handled {
		t.Fatal("incomplete JSON-RPC envelope was misclassified as a response")
	}
}
