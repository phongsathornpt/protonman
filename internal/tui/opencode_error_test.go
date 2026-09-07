package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

func TestClassifyOpenCodeError_Cancellation(t *testing.T) {
	err := context.Canceled
	classified := ClassifyOpenCodeError(err, "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindCancelled {
		t.Fatalf("expected ErrorKindCancelled, got %v", classified.Kind)
	}
	if !classified.Retryable {
		t.Fatal("expected cancellation to be retryable")
	}
}

func TestClassifyOpenCodeError_UnresolvedToolCall(t *testing.T) {
	err := fmt.Errorf("turn failed: %w: model requested another tool", applicationturn.ErrUnresolvedToolCall)
	classified := ClassifyOpenCodeError(err, "opencode", "model")
	if classified.Kind != ErrorKindToolDispatch {
		t.Fatalf("expected ErrorKindToolDispatch, got %v", classified.Kind)
	}
	if classified.Badge != "TOOL_PROTOCOL" {
		t.Fatalf("badge = %q, want TOOL_PROTOCOL", classified.Badge)
	}
	if !strings.Contains(classified.RawDetails, "unresolved model tool call") {
		t.Fatalf("raw details = %q, want sentinel details", classified.RawDetails)
	}
}

func TestClassifyOpenCodeError_ToolDispatchUnavailable(t *testing.T) {
	err := fmt.Errorf("turn failed: %w: model requested 1 tool call while no tools were available", applicationturn.ErrToolDispatchUnavailable)
	classified := ClassifyOpenCodeError(err, "opencode", "model")
	if classified.Kind != ErrorKindToolDispatch {
		t.Fatalf("expected ErrorKindToolDispatch, got %v", classified.Kind)
	}
	if classified.Badge != "TOOL_DISPATCH" {
		t.Fatalf("badge = %q, want TOOL_DISPATCH", classified.Badge)
	}
	if !strings.Contains(classified.Message, "no tools were available") {
		t.Fatalf("message = %q, want unavailable-tool detail", classified.Message)
	}
}

func TestClassifyOpenCodeError_OpenCodeModelError(t *testing.T) {
	raw := `provider returned status 401: {"type":"error","error":{"type":"ModelError","message":"Model nonexistent is not supported"}}`
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nonexistent")

	if classified.Kind != ErrorKindModelNotFound {
		t.Fatalf("expected ErrorKindModelNotFound, got %v", classified.Kind)
	}
	if !strings.Contains(classified.Message, "nonexistent") {
		t.Fatalf("expected message to mention nonexistent, got %q", classified.Message)
	}
	if len(classified.Suggestions) == 0 {
		t.Fatal("expected suggestions to be populated")
	}
	if !strings.Contains(classified.Suggestions[0], "/model") {
		t.Fatalf("expected catalog refresh suggestion, got %q", classified.Suggestions[0])
	}
}

func TestClassifyOpenCodeError_ContextOverflow(t *testing.T) {
	tests := []string{
		"provider returned status 413: request entity too large",
		`provider returned status 400: {"error":{"code":"context_length_exceeded","message":"Input exceeds context window"}}`,
		"model error: prompt is too long; exceeded max context length of 128000 tokens",
		"maximum context length is 128000 tokens, but your request resulted in 130000 tokens",
		"tokens in request more than max tokens allowed",
	}

	for _, raw := range tests {
		classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
		if classified.Kind != ErrorKindContextOverflow {
			t.Errorf("for %q: expected ErrorKindContextOverflow, got %v", raw, classified.Kind)
		}
		if len(classified.Suggestions) == 0 {
			t.Errorf("for %q: expected suggestions for context overflow", raw)
		}
		foundCompact := false
		for _, s := range classified.Suggestions {
			if strings.Contains(s, "/compact") {
				foundCompact = true
				break
			}
		}
		if !foundCompact {
			t.Errorf("for %q: expected suggestion mentioning /compact", raw)
		}
	}
}

func TestClassifyOpenCodeError_Authentication(t *testing.T) {
	raw := "provider returned status 401: unauthorized: invalid api key"
	classified := ClassifyOpenCodeError(errors.New(raw), "openai", "gpt-4o")
	if classified.Kind != ErrorKindAuthentication {
		t.Fatalf("expected ErrorKindAuthentication, got %v", classified.Kind)
	}
	if classified.Code != "401" {
		t.Fatalf("expected code 401, got %q", classified.Code)
	}
}

func TestClassifyOpenCodeError_HTMLGateway(t *testing.T) {
	raw := "provider returned status 401: <!DOCTYPE html><html><head><title>401 Authorization Required</title></head><body><h1>401 Authorization Required</h1></body></html>"
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindAuthentication {
		t.Fatalf("expected ErrorKindAuthentication, got %v", classified.Kind)
	}
	if !strings.Contains(classified.Message, "upstream gateway or proxy") {
		t.Fatalf("expected clean gateway message, got %q", classified.Message)
	}
}

func TestClassifyOpenCodeError_Forbidden(t *testing.T) {
	raw := "provider returned status 403: access_denied for requested resource"
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "claude-sonnet-4")
	if classified.Kind != ErrorKindForbidden {
		t.Fatalf("expected ErrorKindForbidden, got %v", classified.Kind)
	}
}

func TestClassifyOpenCodeError_RateLimitAndQuota(t *testing.T) {
	rawRate := "provider returned status 429: rate limit exceeded. please wait 10 seconds"
	cRate := ClassifyOpenCodeError(errors.New(rawRate), "opencode", "nemotron-3.5-lightning-free")
	if cRate.Kind != ErrorKindRateLimit {
		t.Fatalf("expected ErrorKindRateLimit, got %v", cRate.Kind)
	}

	rawQuota := `provider returned status 429: {"error":{"code":"insufficient_quota","message":"You have exceeded your current quota"}}`
	cQuota := ClassifyOpenCodeError(errors.New(rawQuota), "openai", "gpt-4o")
	if cQuota.Kind != ErrorKindQuotaExceeded {
		t.Fatalf("expected ErrorKindQuotaExceeded, got %v", cQuota.Kind)
	}
}

func TestClassifyOpenCodeError_ServerOverloaded(t *testing.T) {
	raw := "provider returned status 503: Upstream request failed"
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindServerOverloaded {
		t.Fatalf("expected ErrorKindServerOverloaded, got %v", classified.Kind)
	}
	if !classified.Retryable {
		t.Fatal("expected server overloaded to be retryable")
	}
}

func TestClassifyOpenCodeError_Timeout(t *testing.T) {
	raw := "ProviderHeaderTimeoutError: headers timed out after 30000ms"
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindStreamTimeout {
		t.Fatalf("expected ErrorKindStreamTimeout, got %v", classified.Kind)
	}
}

func TestClassifyOpenCodeError_MCPFailed(t *testing.T) {
	raw := `MCP server "weather" failed to start`
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindMCPFailed {
		t.Fatalf("expected ErrorKindMCPFailed, got %v", classified.Kind)
	}
	if !strings.Contains(classified.Message, "weather") {
		t.Fatalf("expected message to mention weather, got %q", classified.Message)
	}
}

func TestClassifyOpenCodeError_ConfigErrors(t *testing.T) {
	raw := `ConfigDirectoryTypoError: Directory "prompt" in /path is not valid. Rename the directory to "prompts"`
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindConfigTypo {
		t.Fatalf("expected ErrorKindConfigTypo, got %v", classified.Kind)
	}
}
