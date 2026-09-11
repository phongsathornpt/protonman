# Protonman SDK

`proton-sdk` is the provider-neutral model layer used by Protonman Agent CLI.
It follows the core provider/model concepts of modern AI SDKs while keeping
Protonman-specific permission, session, and tool execution policy outside the SDK.

## Ownership

`proton-sdk` owns:

- `LanguageModel` and streaming model requests
- model messages, multimodal content, tool calls, and tool results
- normalized usage and finish reasons
- model capabilities, provider options, provider metadata, and optional raw chunks
- structured provider errors
- model registry and language-model middleware
- OpenAI-compatible and Anthropic Messages wire adapters

Protonman CLI owns turn orchestration, permissions, tool execution, sessions,
workspace policy, subagent lifecycle, and TUI/headless presentation.
## Agent Model Contract

Providers implement:

```go
type LanguageModel interface {
    Provider() string
    ModelID() string
    Capabilities() ModelCapabilities
    Stream(context.Context, Request) (Stream, error)
}
```

A stream terminates with `EventFinish`; reaching EOF first is an incomplete
stream error. Tool-call input can stream incrementally before the normalized
complete `EventToolCall` is emitted.

`Request.Options.MaxOutputTokens` is mapped to the provider wire format.
`ProviderOptions["openai"]` and `ProviderOptions["anthropic"]` can add
provider-specific top-level options, but cannot override canonical request
fields such as model, messages/input, tools, stream, or token limits. Tool-level
provider options are also supported, for example Anthropic cache-control metadata.

`ModelCapabilities` lets the agent runtime reject unsupported vision input and
avoid publishing tools to models that do not support tool calling. MCP tools are
marked dynamic when they cross the SDK boundary.

Set `Request.Options.IncludeRawChunks` to receive `EventRaw` before normalized
stream events. Raw chunks are disabled by default and are intended for debugging,
telemetry, and provider-specific integrations.
## Tool Results and Errors

Tools may declare an optional `OutputSchema`. Structured tool output is validated
against JSON Schema before it is returned to the model. External schema loading
is disabled, so untrusted `$ref` values cannot trigger filesystem or network
fetches. MCP output schemas and structured output are preserved through the tool
boundary.

Tool execution remains in Protonman CLI. The live model history records whether a
tool result is an error so Anthropic can emit `tool_result.is_error`; OpenAI
receives the same normalized tool-result content through its protocol shape.
Persisted sessions intentionally compact tool protocol groups to plain assistant
history and do not persist tool arguments.

Provider failures use `ProviderError` with a normalized `ErrorKind`, HTTP status,
provider code, message, and retryability. HTTP and streaming errors use the same
error type, so callers can use `errors.As` instead of parsing provider strings.

Retry and bounded backoff policy belongs to the SDK/provider boundary rather than the
turn engine. `RetryPolicy.RetryDelays` defines the exact 1-based local retry schedule;
the SDK's zero-value policy uses its default schedule, while custom policies without an
explicit schedule retain the bounded exponential fallback. Provider `Retry-After`
metadata still takes precedence over local timing.
Callers that need user-visible progress can attach `WithRetryObserver` to the request
context; `RetryEvent` reports provider/model, reason, retry index/budget, phase
(`waiting` or `cooldown`), delay, and the exact `RetryAt` deadline before the wait
begins. Observers are additive and do not alter retry policy. Model streams must be
closed exactly once on success and on every failure/cancellation path; when processing
and close both fail, preserve the original processing failure as the primary error.

## Providers

- `proton-sdk/provider/openai`: Chat Completions, Responses API, OpenAI-compatible gateways, streaming tools, images, usage, and retries.
- `proton-sdk/provider/anthropic`: Messages API, system mapping, images, `tool_use` / `tool_result`, streaming tool input, usage, and retries.

The CLI provider registry selects the protocol from provider configuration.
Model discovery is protocol-aware and does not embed fallback model catalogs.
