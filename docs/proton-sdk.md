# Proton SDK

`proton-sdk` is the provider-neutral model layer used by Proton Agent CLI.
It follows the core provider/model concepts of modern AI SDKs while keeping
Proton-specific permission, session, and tool execution policy outside the SDK.

## Ownership

`proton-sdk` owns:

- `LanguageModel` and streaming model requests
- model messages, multimodal content, tool calls, and tool results
- normalized usage and finish reasons
- provider options and provider metadata
- structured provider errors
- model registry and language-model middleware
- OpenAI-compatible and Anthropic Messages wire adapters

Proton CLI owns turn orchestration, permissions, tool execution, sessions,
workspace policy, subagent lifecycle, and TUI/headless presentation.
## Agent Model Contract

Providers implement:

```go
type LanguageModel interface {
    Provider() string
    ModelID() string
    Stream(context.Context, Request) (Stream, error)
}
```

A stream terminates with `EventFinish`; reaching EOF first is an incomplete
stream error. Tool-call input can stream incrementally before the normalized
complete `EventToolCall` is emitted.

`Request.Options.MaxOutputTokens` is mapped to the provider wire format.
`ProviderOptions["openai"]` and `ProviderOptions["anthropic"]` can add
provider-specific top-level options, but cannot override canonical request
fields such as model, messages/input, tools, stream, or token limits.
## Tool Results and Errors

Tool execution remains in Proton CLI. The live model history records whether a
tool result is an error so Anthropic can emit `tool_result.is_error`; OpenAI
receives the same normalized tool-result content through its protocol shape.
Persisted sessions intentionally compact tool protocol groups to plain assistant
history and do not persist tool arguments.

Provider failures use `ProviderError` with a normalized `ErrorKind`, HTTP status,
provider code, message, and retryability. HTTP and streaming errors use the same
error type, so callers can use `errors.As` instead of parsing provider strings.

## Providers

- `proton-sdk/provider/openai`: Chat Completions, Responses API, OpenAI-compatible gateways, streaming tools, images, usage, and retries.
- `proton-sdk/provider/anthropic`: Messages API, system mapping, images, `tool_use` / `tool_result`, streaming tool input, usage, and retries.

The CLI provider registry selects the protocol from provider configuration.
Model discovery is protocol-aware and does not embed fallback model catalogs.
