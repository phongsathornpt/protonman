package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/app"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

// classifyCase is one frozen classification input. The corpus deliberately mixes
// sentinel errors, provider wire formats, and free-form text so that the ordered
// rule precedence in Classify is pinned by golden output instead of prose.
type classifyCase struct {
	name     string
	err      error
	provider string
	model    string
}

// overflowSamples holds one realistic message per context-overflow pattern.
// Keeping them as data makes it obvious when a pattern loses its coverage.
var overflowSamples = []string{
	"prompt is too long: 200000 tokens > 128000 maximum",
	"request_too_large",
	"Input is too long for requested model.",
	"The input length exceeds the context window for this model.",
	"This request exceeds the model's maximum context length of 128,000 tokens.",
	"This request exceeds maximum context length (128000).",
	"input token count (130000) exceeds the maximum allowed",
	"Tokens in request more than max tokens allowed",
	"Maximum prompt length is 8192",
	"Please reduce the length of the messages or conversations",
	"maximum context length is 4096 tokens, however you requested 8000 tokens",
	"input length exceeds the maximum allowed input length of 32,000 tokens",
	"Input (131072 tokens) is longer than the model's context length (128000 tokens)",
	"The request exceeds the limit of 1000000 characters",
	"total length of prompt + completion exceeds the available context size",
	"requested generation is greater than the context length",
	"context window exceeds limit for this model",
	"Exceeded model token limit",
	"context_length_exceeded",
	"context length exceeded",
	"Request Entity Too Large",
	"The context length is only 2048 tokens",
	"input length (5000) exceeds context length (4096)",
	"Prompt too long; exceeded max context length by 200 tokens",
	"request is too large for model with 8192 maximum context length",
	"prompt has 12,345 tokens, but the configured context size is 8,192 tokens",
	"response truncated: model_context_window_exceeded",
	"Too many tokens in your prompt",
	"Token limit exceeded for this organization",
}

// overflowExclusionSamples must never classify as context overflow even when the
// text also contains overflow wording, because the exclusion rules win first.
var overflowExclusionSamples = []string{
	"Throttling Error: too many tokens",
	"Service Unavailable: prompt is too long",
	"rate limit: prompt is too long",
	"too many requests, input is too long for requested model",
}

func buildClassifyCorpus() []classifyCase {
	cases := []classifyCase{
		{name: "nil-error"},
		{name: "empty-message", err: errors.New("")},
		{name: "cancelled-sentinel", err: context.Canceled},
		{name: "cancelled-wrapped", err: fmt.Errorf("stream aborted: %w", context.Canceled)},
		{name: "cancelled-text", err: errors.New("context canceled by caller")},
		{name: "cancelled-ui", err: errors.New("UICancelledError: escape pressed")},
		{name: "empty-response", err: fmt.Errorf("turn failed: %w", app.ErrEmptyResponse)},
		{name: "empty-response-bare", err: app.ErrEmptyResponse},
		{name: "stream-incomplete", err: fmt.Errorf("read model stream: %w", domain.ErrIncompleteStream)},
		{name: "stream-incomplete-idle", err: fmt.Errorf("%w: stream became idle before completion", domain.ErrIncompleteStream)},
		{name: "stream-incomplete-no-output", err: fmt.Errorf("%w: produced no output before timeout", domain.ErrIncompleteStream)},
		{name: "stream-incomplete-max-duration", err: fmt.Errorf("%w: stream exceeded maximum duration", domain.ErrIncompleteStream)},
		{name: "tool-dispatch-unavailable", err: app.ErrToolDispatchUnavailable},
		{name: "tool-dispatch-wrapped", err: fmt.Errorf("round 2: %w: model requested 3 tool calls", app.ErrToolDispatchUnavailable)},
		{name: "unresolved-tool-call", err: app.ErrUnresolvedToolCall},
		{name: "unresolved-tool-call-wrapped", err: fmt.Errorf("resolve: %w", app.ErrUnresolvedToolCall)},
		{name: "permission-denied", err: errors.New("permission denied for path .protonman/config.json")},
		{name: "permission-question-rejected", err: errors.New("QuestionRejectedError: user declined")},
		{name: "permission-rule", err: errors.New("tool edit specified a rule that denies this write")},
		{name: "auth-401-status", err: errors.New("provider request failed: status 401 unauthorized")},
		{name: "auth-invalid-key", err: errors.New("invalid api key provided")},
		{name: "auth-code-equals", err: errors.New("gateway responded code=401")},
		{name: "forbidden-403", err: errors.New("status: 403")},
		{name: "forbidden-word", err: errors.New("access forbidden: blocked by policy")},
		{name: "forbidden-access-denied", err: errors.New("oauth access_denied from provider")},
		{name: "quota-code", err: errors.New("insufficient_quota for project p-1")},
		{name: "quota-words", err: errors.New("Quota exceeded. Check your plan and billing details.")},
		{name: "ratelimit-429", err: errors.New("status 429")},
		{name: "ratelimit-words", err: errors.New("Rate limit exceeded for this endpoint")},
		{name: "ratelimit-too-many", err: errors.New("Too Many Requests")},
		{name: "server-500", err: errors.New("upstream status 500")},
		{name: "server-502", err: errors.New("status 502 bad gateway")},
		{name: "server-503", err: errors.New("Service Unavailable")},
		{name: "server-504", err: errors.New("status 504 Gateway Timeout")},
		{name: "server-overloaded-flag", err: errors.New("opencode: server_is_overloaded")},
		{name: "server-error-flag", err: errors.New("provider returned server_error")},
		{name: "server-upstream-failed", err: errors.New("Upstream request failed after 3 attempts")},
		{name: "transport-header-timeout", err: errors.New("ProviderHeaderTimeoutError: upstream response timeout")},
		{name: "transport-stream-error", err: errors.New("ProviderResponseStreamError: mid-stream reset")},
		{name: "transport-client-timeout", err: errors.New("client.Timeout exceeded while awaiting headers")},
		{name: "transport-io-timeout", err: errors.New("read tcp: i/o timeout")},
		{name: "transport-tls-timeout", err: errors.New("TLS handshake timeout from dialer")},
		{name: "transport-dial-tcp", err: errors.New("dial tcp 10.0.0.1:443: connect: operation timed out")},
		{name: "transport-unexpected-eof", err: errors.New("unexpected EOF")},
		{name: "transport-connection-reset", err: errors.New("read: connection reset by peer")},
		{name: "runtime-deadline-sentinel", err: context.DeadlineExceeded},
		{name: "runtime-deadline-wrapped", err: fmt.Errorf("worker wait failed: %w", context.DeadlineExceeded)},
		{name: "runtime-deadline-text", err: errors.New("tool execution deadline exceeded")},
		{name: "runtime-timed-out", err: errors.New("subagent run timed out after 30m0s")},
		{name: "runtime-timeout-word", err: errors.New("timeout waiting for permission response")},
		{name: "mcp-failed-flag", err: errors.New("MCPFailed: transport closed")},
		{name: "mcp-server-generic", err: errors.New("MCP server unavailable")},
		{name: "mcp-server-named-double", err: errors.New(`MCP server "github" failed to initialize`)},
		{name: "mcp-server-named-single", err: errors.New("MCP server 'weather' failed after 3 retries")},
		{name: "mcp-server-named-bare", err: errors.New("MCP server fetch failed: DNS error")},
		{name: "config-typo-flag", err: errors.New("ConfigDirectoryTypoError: ~/.opencode is not valid. Rename the directory")},
		{name: "config-typo-words", err: errors.New("directory .openecode is not valid. Rename the directory")},
		{name: "config-json", err: errors.New("ConfigJsonError: invalid character '}' looking for beginning of value")},
		{name: "config-invalid", err: errors.New("ConfigInvalidError: unknown field \"model\"")},
		{name: "config-invalid-words", err: errors.New("Configuration is invalid: provider missing baseUrl")},
		{name: "tool-failed-words", err: errors.New("tool execution failed: bash")},
		{name: "tool-failed-command", err: errors.New("[COMMAND_FAILED] exit status 1")},
		{name: "tool-failed-notfound", err: errors.New("[FILE_NOT_FOUND] internal/nope.go")},
		{name: "model-not-supported", err: errors.New("model nemotron-3.5-lightning-free is not supported by provider opencode"), provider: "opencode", model: "fallback-model"},
		{name: "model-not-supported-no-provider", err: errors.New("model gpt-9 is not supported"), model: "gpt-5"},
		{name: "model-provider-not-found", err: errors.New("ProviderModelNotFoundError: requested model unavailable"), provider: "anthropic"},
		{name: "model-quoted-not-found", err: errors.New("model 'claude-y' not found in catalog"), provider: "openai", model: "gpt-5"},
		{name: "model-does-not-exist", err: errors.New("model deepseek-v4 does not exist"), provider: "opencode"},
		{name: "model-404-with-word", err: errors.New("status 404: no matching model for this request"), provider: "opencode"},
		{name: "notfound-404-no-model", err: errors.New("status 404: workspace resource missing"), provider: "opencode"},
		{name: "context-413", err: errors.New("request failed with status 413"), provider: "opencode"},
		{name: "generic-turn-prefix", err: errors.New("turn failed: something completely unexpected"), provider: "opencode", model: "muse-spark"},
		{name: "generic-multibyte", err: errors.New("完全未知的錯誤：模型拒絕回應")},
		{name: "generic-emoji", err: errors.New("provider shrugged 🤷 no known code")},
		{name: "generic-long-tail", err: errors.New("unknown failure " + strings.Repeat("padding-token ", 400))},
		{name: "status-four-digits", err: errors.New("provider code 4041 rejected")},
		{name: "overflow-rate-limit-exclusion", err: errors.New("rate limit: prompt is too long")},
		{name: "overflow-parsed-message-beats-exclusion", err: errors.New("429 rate limited\n{\"type\":\"error\",\"error\":{\"type\":\"BadRequest\",\"message\":\"prompt is too long\"}}")},
	}

	for _, body := range []string{
		`{"type":"error","error":{"type":"ModelError","message":"Model foo is not supported"}}`,
		`{"type":"error","error":{"type":"AuthError","message":"Unauthorized"}}`,
		`{"type":"error","error":{"type":"RateLimitError","message":"Slow down"}}`,
		`{"type":"error","error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"too large"}}`,
		`{"error":{"message":"Invalid API key","type":"authentication_error","code":"invalid_api_key"}}`,
		`{"error":{"message":"Quota exceeded.","type":"insufficient_quota","code":429}}`,
		`{"error":{"message":"Context Window Overflow Detected","type":"request_too_large","code":"context_length_exceeded"}}`,
		`{"error":{"message":"","type":"empty_message"}}`,
		`{"message":"Something went sideways"}`,
		`{"detail":"Bad thing happened"}`,
		`{"error":"plain string error"}`,
		`{"error":`,
		`{not json}`,
		`[]`,
		`"just a string"`,
		`prefix {"error":{"message":"inner payload"}} suffix`,
		`{"error":{"message":"[FILE_NOT_FOUND] missing","type":"invalid_request_error"}}`,
		`{"error":{"message":"permission denied while reading config","type":"permission_error"}}`,
	} {
		cases = append(cases, classifyCase{
			name:     "payload-" + sanitizeCaseName(body),
			err:      fmt.Errorf("provider rejected request: %s", body),
			provider: "opencode",
			model:    "nemotron-3.5-lightning-free",
		})
	}

	for _, body := range []string{
		`<!doctype html><html><title>401 Unauthorized</title>status 401`,
		`<html><body>Forbidden</body></html> status: 403`,
		`<!DOCTYPE html><html>cloudflare</html> status 500`,
		`<html><body>no status marker</body></html>`,
	} {
		cases = append(cases, classifyCase{
			name:     "gateway-" + sanitizeCaseName(body),
			err:      errors.New(body),
			provider: "opencode",
		})
	}

	for index, sample := range overflowSamples {
		cases = append(cases, classifyCase{
			name:     fmt.Sprintf("overflow-%02d", index),
			err:      errors.New(sample),
			provider: "opencode",
			model:    "muse-spark",
		})
	}
	for index, sample := range overflowExclusionSamples {
		cases = append(cases, classifyCase{
			name:     fmt.Sprintf("overflow-excluded-%02d", index),
			err:      errors.New(sample),
			provider: "opencode",
		})
	}
	return cases
}

// buildLongOverflowSample is a synthetic worst case: a very long message whose
// only match sits near the end of the pattern scan. It keeps the classifier
// honest about linear rather than quadratic behavior on large provider payloads.
func buildLongOverflowSample() error {
	return errors.New(strings.Repeat("0123456789", 2000) + " prompt has 12,345 tokens, but the configured context size is 8,192 tokens")
}

func sanitizeCaseName(value string) string {
	replacer := strings.NewReplacer(" ", "-", "\n", "-", "\t", "-", "\"", "'", "{", "", "}", "", ":", "", "/", "-")
	name := replacer.Replace(value)
	if len(name) > 48 {
		name = name[:48]
	}
	return strings.Trim(name, "-")
}
