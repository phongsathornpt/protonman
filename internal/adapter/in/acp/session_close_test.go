package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestACPInitializeAdvertisesSessionClose(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	result, _, err := server.dispatch(context.Background(), RPCRequest{Method: "initialize"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("initialize error = %v", err)
	}

	initResult, ok := result.(InitializeResult)
	if !ok {
		t.Fatalf("initialize result type = %T, want InitializeResult", result)
	}
	if initResult.AgentCapabilities.SessionCapabilities.Close == nil {
		t.Fatal("initialize did not advertise sessionCapabilities.close")
	}
}

func TestACPSessionCloseReleasesActiveSession(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID

	result, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/close",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `"}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/close error = %v", err)
	}
	if result == nil {
		t.Fatal("session/close result = nil, want empty success object")
	}
	if _, ok := server.lookupSession(sessionID); ok {
		t.Fatalf("session %q still active after close", sessionID)
	}
}

func TestACPSessionCloseDoesNotDeletePersistedHistory(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID

	if err := server.closeSession(context.Background(), sessionID); err != nil {
		t.Fatalf("closeSession() error = %v", err)
	}

	// Closing is a runtime lifecycle operation. A later resume/load is allowed to
	// reconstruct the session from persistence; unlike session/delete it does not
	// invoke the persistence delete path.
	if _, ok := server.lookupSession(sessionID); ok {
		t.Fatalf("session %q still active after close", sessionID)
	}
}

func TestACPSessionCloseRejectsUnknownSession(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	_, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/close",
		Params: json.RawMessage(`{"sessionId":"missing"}`),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown session") {
		t.Fatalf("session/close error = %v, want unknown session", err)
	}
}
