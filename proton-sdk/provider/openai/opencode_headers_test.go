package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestOpenCodeRequestMetadataHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := map[string]string{
			"x-opencode-session":  "session-1",
			"x-opencode-project":  "project-1",
			"x-opencode-request":  "msg_message-1",
			"x-opencode-client":   "protonman",
			"x-parent-session-id": "parent-1",
			"User-Agent":          "Protonman-Test",
		}
		for key, expected := range want {
			if got := r.Header.Get(key); got != expected {
				t.Errorf("%s = %q, want %q", key, got, expected)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	provider := NewProvider(ProviderOptions{
		ProviderName: "opencode",
		BaseURL:      server.URL,
		UserAgent:    "Protonman-Test",
		Headers:      http.Header{"x-opencode-client": []string{"protonman"}},
	})
	stream, err := provider.Model("test-model").Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{ID: "msg_message-1", Role: sdk.RoleUser, Content: "hi"}},
		Metadata: sdk.RequestMetadata{
			SessionID:       "session-1",
			ParentSessionID: "parent-1",
			ProjectID:       "project-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestOpenCodeRequestHeaderUsesExplicitRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-request"); got != "request-override" {
			t.Errorf("x-opencode-request = %q, want %q", got, "request-override")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{ID: "msg_message-1", Role: sdk.RoleUser, Content: "hi"}},
		Metadata: sdk.RequestMetadata{SessionID: "session-1", RequestID: "request-override"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestNonOpenCodeProviderDoesNotReceiveOpenCodeHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, key := range []string{"x-opencode-session", "x-opencode-project", "x-opencode-request"} {
			if got := r.Header.Get(key); got != "" {
				t.Errorf("%s = %q, want empty", key, got)
			}
		}
		if got := r.Header.Get("X-Session-Id"); got != "session-1" {
			t.Errorf("X-Session-Id = %q, want %q", got, "session-1")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{ProviderName: "custom", BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{ID: "msg_message-1", Role: sdk.RoleUser, Content: "hi"}},
		Metadata: sdk.RequestMetadata{SessionID: "session-1", ProjectID: "project-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}
