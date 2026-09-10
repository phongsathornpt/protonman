package model

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestOpenCodeSDKAdapterAlwaysSendsClientIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-client"); got != "proton-test" {
			t.Fatalf("x-opencode-client = %q", got)
		}
		if got := r.Header.Get("X-Agent-Type"); got != "protonman" {
			t.Fatalf("X-Agent-Type = %q", got)
		}
		if got := r.Header.Get("X-Agent-Version"); got == "" {
			t.Fatal("X-Agent-Version is empty")
		}
		if got := r.Header.Get("x-opencode-session"); got != "" {
			t.Fatalf("x-opencode-session = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, server.URL, "", "test", WithClientName("proton-test"))
	if model.Provider() != DefaultOpenCodeName {
		t.Fatalf("provider = %q", model.Provider())
	}
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestOpenCodeSDKAdapterSendsSessionIdentityWhenPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-client"); got != "proton" {
			t.Fatalf("x-opencode-client = %q", got)
		}
		if got := r.Header.Get("x-opencode-session"); got != "session-1" {
			t.Fatalf("x-opencode-session = %q", got)
		}
		if got := r.Header.Get("X-Session-Id"); got != "session-1" {
			t.Fatalf("X-Session-Id = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, server.URL, "", "test", WithSessionID("session-1"))
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestAnthropicSDKAdapterSendsSessionIdentityWhenPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Session-Id"); got != "session-anthropic" {
			t.Fatalf("X-Session-Id = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	model := newSDKAnthropicLanguageModel(server.URL, "", "claude-test", WithSessionID(" session-anthropic "))
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Metadata: sdk.RequestMetadata{SessionID: "caller-session"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestAgentHeadersIncludeProfile(t *testing.T) {
	cfg := newClientConfig("https://example.com", "", "test")
	WithAgentProfile(" intelligence ")(&cfg)
	headers := agentHeaders(cfg)
	if got := headers.Get("X-Agent-Type"); got != "protonman" {
		t.Fatalf("X-Agent-Type = %q", got)
	}
	if got := headers.Get("X-Agent-Profile"); got != "intelligence" {
		t.Fatalf("X-Agent-Profile = %q", got)
	}
	if got := headers.Get("X-Agent-Version"); got == "" {
		t.Fatal("X-Agent-Version is empty")
	}
}
