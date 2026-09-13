# Protonman SDK

`proton-sdk` is the provider-neutral model layer used by Protonman Agent CLI.
It keeps model protocol concerns inside the SDK while Protonman-specific turn,
permission, session, tool-execution, and subagent policy stays outside it.

## Ownership

`proton-sdk` owns:

- `LanguageModel` and streaming model requests
- model messages, multimodal content, tool calls, and tool results
- normalized responses, usage, and finish reasons
- model capabilities and provider-neutral model metadata
- provider options, provider metadata, and optional raw chunks
- structured provider errors, retry policy, and retry observation
- model registry and language-model middleware
- OpenAI-compatible and Anthropic Messages wire adapters

Protonman runtime owns turn orchestration, termination policy, permissions, tool
execution, sessions, workspace policy, subagent lifecycle, and TUI/headless
presentation. Generic agent-loop stop helpers do not belong in `proton-sdk`;
termination behavior should live with the runtime/turn engine that actually uses it.

## Package Shape

The SDK root intentionally remains a flat Go package so callers continue to use
`protonsdk.Request`, `protonsdk.Message`, `protonsdk.Tool`, and `protonsdk.Event`
without nested `message.Message` or `stream.Event` APIs.

```text
proton-sdk/
  language_model.go
  request.go
  response.go
  message.go
  content.go
  tool.go
  reasoning.go
  stream.go
  usage.go
  metadata.go
  capabilities.go
  collect.go
  history.go
  error.go
  middleware.go
  registry.go
  retry.go
  retry_observer.go
  rate_limit.go
  provider/
    openai/
    anthropic/
```

Provider wire request/response types remain private to their provider package.
The root SDK must not depend on concrete provider packages or agent-loop policy.

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

Canonical event constructors such as `NewTextDeltaEvent`, `NewToolCallEvent`,
`NewUsageEvent`, and `NewFinishEvent` should be preferred over ad-hoc `Event`
struct literals when producing normalized streams. Constructors reduce accidental
mixed event payloads while `Event.Validate` remains the runtime contract check.

## Collected Responses

`Response` is the canonical normalized output collected from a model stream.
Use `Collect` to consume a stream through its terminal event and
`AppendAssistantResponse` when adding the normalized assistant output back to
provider-neutral conversation history.

`StepResult`, `CollectStep`, and `AppendAssistantStep` remain deprecated
compatibility surfaces for existing callers and delegate to the canonical
response API. New code should not introduce additional agent-loop terminology
into `proton-sdk`.

`Request.Options.MaxOutputTokens` is mapped to the provider wire format.
`ProviderOptions["openai"]` and `ProviderOptions["anthropic"]` can add
provider-specific top-level options, but cannot override canonical request
fields such as model, messages/input, tools, stream, or token limits. Tool-level
provider options are also supported, for example Anthropic cache-control metadata.

`ModelCapabilities` represents effective runtime capabilities for the model
instance. Provider implementations may publish protocol defaults while outer
model-profile wrappers narrow them for a concrete model route.

Optional provider-neutral model metadata is exposed through `ModelMetadata` and
`MetadataModel`. Built-in provider models and Protonman model decorators preserve
this boundary end to end. `TokenLimitsModel` and `ContextWindowModel` remain
legacy compatibility surfaces while callers migrate to the canonical metadata
API. Middleware must preserve canonical metadata when wrapping a model.

Set `Request.Options.IncludeRawChunks` to receive `EventRaw` before normalized
stream events. Raw chunks are disabled by default and are intended for debugging,
telemetry, and provider-specific integrations.

## Provider Configuration

Provider construction uses a package-local `Config` type:

```go
openai.NewProvider(openai.Config{...})
anthropic.NewProvider(anthropic.Config{...})
```

`openai.ProviderOptions` and `anthropic.ProviderOptions` are deprecated
compatibility aliases for existing callers. The root `protonsdk.ProviderOptions`
type is different: it is the per-request provider escape hatch carried in
canonical requests and tools. New provider code should use `Config` for
long-lived construction settings and `protonsdk.ProviderOptions` only for
request-scoped wire extensions.

## Tool Results and Errors

Tools may declare an optional `OutputSchema`. Structured tool output is validated
against JSON Schema before it is returned to the model. External schema loading
is disabled, so untrusted `$ref` values cannot trigger filesystem or network
fetches. MCP output schemas and structured output are preserved through the tool
boundary.

Tool execution remains in Protonman runtime. The live model history records whether
a tool result is an error so Anthropic can emit `tool_result.is_error`; OpenAI
receives the same normalized tool-result content through its protocol shape.
Persisted sessions intentionally compact tool protocol groups to plain assistant
history and do not persist tool arguments.

Provider failures use `ProviderError` with a normalized `ErrorKind`, HTTP status,
provider code, message, and retryability. HTTP and streaming errors use the same
error type, so callers can use `errors.As` instead of parsing provider strings.

Retry and bounded backoff policy belongs to the SDK/provider boundary rather than
the turn engine. `RetryPolicy.RetryDelays` defines the exact 1-based local retry
schedule; the SDK's zero-value policy uses its default schedule, while custom
policies without an explicit schedule retain the bounded exponential fallback.
Provider `Retry-After` metadata still takes precedence over local timing.

Callers that need user-visible progress can attach `WithRetryObserver` to the
request context; `RetryEvent` reports provider/model, reason, retry index/budget,
phase (`waiting` or `cooldown`), delay, and the exact `RetryAt` deadline before the
wait begins. Observers are additive and do not alter retry policy. Model streams
must be closed exactly once on success and on every failure/cancellation path;
when processing and close both fail, preserve the original processing failure as
the primary error.

## Providers

- `proton-sdk/provider/openai`: Chat Completions, Responses API, OpenAI-compatible gateways, streaming tools, images, usage, and retries.
- `proton-sdk/provider/anthropic`: Messages API, system mapping, images, `tool_use` / `tool_result`, streaming tool input, usage, and retries.

The CLI provider registry selects the protocol from provider configuration.
Model discovery is protocol-aware and does not embed fallback model catalogs.
