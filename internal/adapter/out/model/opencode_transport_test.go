package model

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestResolveOpenCodeTransport(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		baseURL   string
		modelID   string
		transport openCodeTransport
	}{
		{name: "zen gpt", provider: DefaultOpenCodeName, baseURL: "https://opencode.ai/zen/v1", modelID: "gpt-5.5", transport: openCodeTransportResponses},
		{name: "zen grok", provider: DefaultOpenCodeName, baseURL: "https://opencode.ai/zen/v1", modelID: "grok-4.6", transport: openCodeTransportResponses},
		{name: "zen muse namespaced", provider: DefaultOpenCodeName, baseURL: "https://opencode.ai/zen/v1", modelID: "oc/muse-spark-1.3-contributor-free", transport: openCodeTransportResponses},
		{name: "zen claude", provider: DefaultOpenCodeName, baseURL: "https://opencode.ai/zen/v1", modelID: "claude-sonnet-5", transport: openCodeTransportMessages},
		{name: "zen qwen", provider: DefaultOpenCodeName, baseURL: "https://opencode.ai/zen/v1", modelID: "qwen3.6-plus", transport: openCodeTransportMessages},
		{name: "zen deepseek", provider: DefaultOpenCodeName, baseURL: "https://opencode.ai/zen/v1", modelID: "deepseek-v4-flash", transport: openCodeTransportChat},
		{name: "zen nemotron", provider: DefaultOpenCodeName, baseURL: "https://opencode.ai/zen/v1", modelID: "nemotron-3.5-lightning-free", transport: openCodeTransportChat},
		{name: "zen unsupported", provider: DefaultOpenCodeName, baseURL: "https://opencode.ai/zen/v1", modelID: "gemini-3.8-flash", transport: openCodeTransportUnsupported},
		{name: "router remains generic", provider: "9router", baseURL: "http://127.0.0.1:20128/v1", modelID: "oc/gpt-5.5", transport: openCodeTransportChat},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveOpenCodeTransport(test.provider, test.baseURL, test.modelID); got != test.transport {
				t.Fatalf("resolveOpenCodeTransport() = %d, want %d", got, test.transport)
			}
		})
	}
}

func TestOpenCodeZenUsesDocumentedRequestEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		path    string
	}{
		{name: "chat", modelID: "nemotron-3.5-lightning-free", path: "/zen/v1/chat/completions"},
		{name: "responses", modelID: "gpt-5.5", path: "/zen/v1/responses"},
		{name: "messages", modelID: "claude-sonnet-5", path: "/zen/v1/messages"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestPath := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestPath <- r.URL.Path
				http.NotFound(w, r)
			}))
			defer server.Close()

			languageModel := NewProviderLanguageModel(
				DefaultOpenCodeName,
				string(ProviderProtocolOpenAI),
				server.URL+"/zen/v1",
				"zen-key",
				test.modelID,
			)
			_, err := languageModel.Stream(context.Background(), sdk.Request{
				Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hello"}},
			})
			if err == nil {
				t.Fatal("expected endpoint fixture to return an error")
			}
			select {
			case got := <-requestPath:
				if got != test.path {
					t.Fatalf("request path = %q, want %q", got, test.path)
				}
			default:
				t.Fatal("provider did not make an HTTP request")
			}
		})
	}
}

func TestOpenCodeZenRejectsUnsupportedModelBeforeRequest(t *testing.T) {
	languageModel := NewProviderLanguageModel(
		DefaultOpenCodeName,
		string(ProviderProtocolOpenAI),
		"https://opencode.ai/zen/v1",
		"zen-key",
		"gemini-3.8-flash",
	)
	_, err := languageModel.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hello"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported OpenCode model transport") {
		t.Fatalf("error = %v, want unsupported transport error", err)
	}
}

func TestOpenCodeRouteLeavesNineRouterModelIDUntouched(t *testing.T) {
	requestPath := make(chan string, 1)
	requestBody := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath <- r.URL.Path
		body, _ := io.ReadAll(r.Body)
		requestBody <- string(body)
		http.NotFound(w, r)
	}))
	defer server.Close()

	languageModel := NewProviderLanguageModel(
		"9router",
		string(ProviderProtocolOpenAI),
		server.URL+"/v1",
		"router-key",
		"oc/gpt-5.5",
	)
	_, err := languageModel.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected endpoint fixture to return an error")
	}
	if got := <-requestPath; got != "/v1/chat/completions" {
		t.Fatalf("request path = %q, want /v1/chat/completions", got)
	}
	if got := <-requestBody; !strings.Contains(got, `"model":"oc/gpt-5.5"`) {
		t.Fatalf("request body did not preserve model ID: %s", got)
	}
}
