package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
)

type requestCancellationRunner struct{}

func (requestCancellationRunner) Run(ctx context.Context, _ []model.Message, _ applicationturn.Sink) (applicationturn.Result, error) {
	<-ctx.Done()
	return applicationturn.Result{}, ctx.Err()
}

func TestACPProtocolCancellationCancelsPromptByRequestID(t *testing.T) {
	server := newTestServerWithRunner(t, permission.ModeAsk, requestCancellationRunner{})
	const sessionID = "request-cancel-session"
	sess, err := server.newSession(context.Background(), sessionID, "/tmp", nil)
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}
	server.mu.Lock()
	server.sessions[sessionID] = sess
	server.mu.Unlock()

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":77,"method":"session/prompt","params":{"sessionId":"request-cancel-session","prompt":[{"type":"text","text":"keep working"}]}}`,
		`{"jsonrpc":"2.0","method":"$/cancel_request","params":{"requestId":77}}`,
	}, "\n") + "\n"
	var output strings.Builder
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	got := output.String()
	if !strings.Contains(got, `"id":77`) || !strings.Contains(got, `"stopReason":"cancelled"`) {
		t.Fatalf("cancelled prompt response missing: %s", got)
	}
	if strings.Contains(got, `"id":77,"error"`) {
		t.Fatalf("cancelled prompt returned an error instead of a valid cancellation result: %s", got)
	}
}

func TestRequestIDKeyPreservesJSONRPCIDType(t *testing.T) {
	numeric, err := requestIDKey(json.RawMessage(`7`))
	if err != nil {
		t.Fatalf("numeric requestIDKey() error = %v", err)
	}
	text, err := requestIDKey(json.RawMessage(`"7"`))
	if err != nil {
		t.Fatalf("string requestIDKey() error = %v", err)
	}
	if numeric == text {
		t.Fatalf("numeric and string request IDs collapsed to the same key %q", numeric)
	}
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`true`), json.RawMessage(`1.5`)} {
		if _, err := requestIDKey(raw); err == nil {
			t.Fatalf("requestIDKey(%s) error = nil, want invalid request id", raw)
		}
	}
}
