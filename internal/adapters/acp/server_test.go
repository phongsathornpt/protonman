package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

func TestACPInitializeAndPrompt(t *testing.T) {
	server := newTestServer(t, permission.ModeAlwaysApprove)
	var output bytes.Buffer
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{}}`,
	}, "\n") + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if !strings.Contains(output.String(), `"protocolVersion":1`) {
		t.Fatalf("initialize missing version: %s", output.String())
	}
	if !strings.Contains(output.String(), `"sessionId"`) {
		t.Fatalf("session/new missing sessionId: %s", output.String())
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

func TestACPCancelStopsInFlightPrompt(t *testing.T) {
	runner := &blockingACPRunner{canceled: make(chan struct{})}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)
	created, _, err := server.dispatch(context.Background(), rpcRequest{Method: "session/new"})
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(sessionNewResult).SessionID

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sessionID + `","prompt":[{"type":"text","text":"wait"}]}}`,
		`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"` + sessionID + `"}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	select {
	case <-runner.canceled:
	default:
		t.Fatal("runner did not observe prompt cancellation")
	}
	if !strings.Contains(output.String(), `"stopReason":"cancelled"`) {
		t.Fatalf("cancelled prompt response = %s", output.String())
	}
	if strings.Contains(output.String(), `"session/update"`) {
		t.Fatalf("cancelled prompt emitted completed content: %s", output.String())
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
	for _, line := range bytes.Split(raw, []byte("\n")) {
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
	canceled chan struct{}
}

func (r *blockingACPRunner) Run(ctx context.Context, _ []model.Message, _ applicationturn.Sink) (applicationturn.Result, error) {
	<-ctx.Done()
	close(r.canceled)
	return applicationturn.Result{}, ctx.Err()
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
	server, err := New(service, registry, runner)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return server
}
