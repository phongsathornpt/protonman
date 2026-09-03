package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/projectTHORN/proton/internal/sandbox"
)

func TestWebFetchHonorsBlockedNetworkPolicy(t *testing.T) {
	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkBlocked})
	_, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-1", "web_fetch", map[string]any{
		"url": "https://example.com",
	}))
	if !errors.Is(err, sandbox.ErrNetworkDenied) {
		t.Fatalf("Execute() error = %v, want network denied", err)
	}
}

func TestWebFetchReadsAllowedLocalServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok-body"))
	}))
	t.Cleanup(server.Close)

	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted})
	result, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-2", "web_fetch", map[string]any{
		"url": server.URL,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "ok-body" {
		t.Fatalf("output = %q, want ok-body", result.Output)
	}
}

func TestWebFetchDefinitionKind(t *testing.T) {
	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted})
	if handler.Definition().Kind != "web_fetch" {
		t.Fatalf("kind = %s", handler.Definition().Kind)
	}
	if _, err := json.Marshal(handler.Definition().InputSchema); err != nil {
		t.Fatalf("schema marshal error = %v", err)
	}
}
