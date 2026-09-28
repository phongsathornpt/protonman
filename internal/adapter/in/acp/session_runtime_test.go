package acp

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/sessionfs"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func TestNormalizeSessionRuntime(t *testing.T) {
	got := normalizeSessionRuntime(SessionRuntimeSettings{
		Provider:       " Protonman ",
		Model:          " qwen3.8-27b ",
		Reasoning:      " HIGH ",
		LowConcurrency: " enabled ",
	})
	if got.Provider != "Protonman" || got.Model != "qwen3.8-27b" {
		t.Fatalf("selection = %#v", got)
	}
	if got.Reasoning != "high" {
		t.Fatalf("reasoning = %q, want high", got.Reasoning)
	}
	if got.LowConcurrency != "on" {
		t.Fatalf("low concurrency = %q, want on", got.LowConcurrency)
	}
}

func TestReasoningSettingUsesAutoForDefault(t *testing.T) {
	if got := reasoningSetting(domain.ReasoningDefault); got != "auto" {
		t.Fatalf("reasoning default = %q, want auto", got)
	}
	if got := reasoningSetting(domain.ReasoningHigh); got != "high" {
		t.Fatalf("reasoning high = %q, want high", got)
	}
}

func TestSetSessionReasoningRejectsActivePrompt(t *testing.T) {
	server := &Server{}
	sess := &Session{id: "s1", active: true}

	err := server.setSessionReasoning(context.Background(), sess, domain.ReasoningHigh)
	if err == nil || !strings.Contains(err.Error(), "active prompt") {
		t.Fatalf("setSessionReasoning error = %v, want active prompt rejection", err)
	}
}

func TestRuntimeSessionLoadsUnactivatedSessionFromStorage(t *testing.T) {
	ctx := context.Background()
	store, err := sessionfs.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	sessionID := "unloaded-session-1"
	if err := store.Save(ctx, sessionID, session.State{
		SessionID:      sessionID,
		ModelProvider:  "runanyware",
		ModelID:        "mimo-v2.6-pro",
		PermissionMode: "ask",
	}); err != nil {
		t.Fatalf("store.Save: %v", err)
	}

	server := newTestServer(t, permission.ModeAlwaysApprove)
	server.sessionService = app.NewSessions(store)
	sessionRuntimeControls.Store(server, sessionRuntimeControl{
		defaults: SessionRuntimeSettings{Provider: "runanyware", Model: "mimo-v2.6-pro"},
		build: func(ctx context.Context, sID, cwd string, svc *toolcall.Service, a app.Agents, set SessionRuntimeSettings) (app.Conversation, error) {
			return testMockConversation{}, nil
		},
	})

	sess, err := server.runtimeSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("runtimeSession error = %v, want successful session load", err)
	}
	if sess == nil || sess.id != sessionID {
		t.Fatalf("runtimeSession returned invalid session: %#v", sess)
	}

	result, handled, err := server.dispatchSessionRuntime(ctx, RPCRequest{
		Method: methodSessionRuntime,
		Params: []byte(`{"sessionId":"` + sessionID + `"}`),
	})
	if err != nil || !handled {
		t.Fatalf("dispatchSessionRuntime error = %v, handled = %v", err, handled)
	}
	runtimeResult, ok := result.(ProtonmanSessionRuntimeResult)
	if !ok {
		t.Fatalf("result type = %T, want ProtonmanSessionRuntimeResult", result)
	}
	if runtimeResult.Model != "mimo-v2.6-pro" {
		t.Fatalf("runtimeResult.Model = %q, want mimo-v2.6-pro", runtimeResult.Model)
	}

	setModelResult, handled, err := server.dispatchSessionRuntime(ctx, RPCRequest{
		Method: methodSessionSetModel,
		Params: []byte(`{"sessionId":"` + sessionID + `","model":"deepseek-v4.1-flash"}`),
	})
	if err != nil || !handled {
		t.Fatalf("dispatchSessionRuntime set_model error = %v, handled = %v", err, handled)
	}
	setResult, ok := setModelResult.(ProtonmanSessionRuntimeResult)
	if !ok || setResult.Model != "deepseek-v4.1-flash" {
		t.Fatalf("set_model result = %#v, want model deepseek-v4.1-flash", setModelResult)
	}

	setProtonmanResult, handled, err := server.dispatchSessionRuntime(ctx, RPCRequest{
		Method: methodSessionSetModel,
		Params: []byte(`{"sessionId":"` + sessionID + `","provider":"protonman","model":"proton/muse-spark-1.3-contributor"}`),
	})
	if err != nil || !handled {
		t.Fatalf("dispatchSessionRuntime set_model with protonman error = %v, handled = %v", err, handled)
	}
	protonmanResult, ok := setProtonmanResult.(ProtonmanSessionRuntimeResult)
	if !ok || protonmanResult.Provider != "protonman" || protonmanResult.Model != "proton/muse-spark-1.3-contributor" {
		t.Fatalf("set_model with protonman result = %#v, want provider protonman and model proton/muse-spark-1.3-contributor", setProtonmanResult)
	}
}

type testMockConversation struct{}

func (testMockConversation) Run(ctx context.Context, messages []domain.Message, sink app.Sink) (app.Result, error) {
	return app.Result{}, nil
}
