# AGENTS.md

## Purpose

This file is the working contract for coding agents modifying Protonman.
It summarizes the current codebase, architectural ownership, runtime invariants,
and the expected engineering workflow. Treat repository code and tests as the
source of truth when this document and implementation ever disagree.

Protonman is a Go 1.27 autonomous coding agent with three inbound modes:

- Bubble Tea interactive TUI
- headless CLI for scripts and CI
- ACP stdio server for editor/IDE integrations

The project prioritizes clean architecture, explicit capability boundaries,
fail-closed security, bounded concurrency, structured tool contracts, and
empirical verification.

## Engineering Contract

When changing Protonman:

1. Inspect the relevant implementation and nearby tests before editing.
2. Preserve Clean Architecture dependency direction.
3. Prefer the smallest coherent fix over broad speculative refactors.
4. Preserve unrelated user work and repository state.
5. Add or update tests for behavioral changes.
6. Run the narrowest useful verifier first, then broaden according to risk.
7. Do not claim verification that did not actually run.
8. Do not create commits, branches, releases, or deployments unless explicitly requested.

## Repository Shape

```text
cmd/protonman/                     composition root and CLI mode selection
internal/adapter/in/            inbound adapters
  acp/                          ACP JSON-RPC/stdin-stdout adapter
  headless/                     non-interactive CLI adapter
  tui/                          Bubble Tea presentation layer
internal/adapter/out/           driven infrastructure adapters
  config/                       layered TOML loading and persistence
  model/                        provider discovery and SDK adaptation
  sessionfs/                    file-backed session repository
  tool/
    agent/                      subagent lifecycle tools and capability registry
    builtin/                    workspace coding tools
    mcp/                        external MCP discovery and handlers
    skill/                      skill activation tool
    todo/                       session-bound task tools
    web/                        web fetch adapter
internal/app/                   application use-case boundaries
internal/base/                  leaf utilities with zero internal dependencies
internal/core/                  pure domain contracts and policies
internal/engine/                prompt, tool-call, and turn orchestration
internal/feature/               agent, project, skill, and todo features
internal/platform/              checkpoint, sandbox, and telemetry infrastructure
proton-sdk/                     provider-neutral model SDK
proton-sdk/provider/            OpenAI-compatible and Anthropic protocol implementations
test/architecture/              dependency and structure guards
test/e2e/                       binary/provider/TUI integration tests
test/unit/                      application/domain unit tests
```

## Clean Architecture Boundaries

Dependency direction is enforced by `test/architecture/dependency_test.go`.
Do not weaken those tests to make an architectural violation pass.

### `internal/base/*`

Leaf packages only. They must not import any other `internal/*` or `cmd/*` package.
Current responsibilities include build metadata, context helpers, environment
parsing, failure classification, glob matching, and global runtime defaults.

### `internal/core/*`

Pure domain contracts and policies:

- `conversation`: provider-neutral conversation retention and historical tool-message policy
- `modelprofile`: model metadata/capability policy
- `permission`: permission modes, rules, grants, request evaluation
- `session`: session aggregate, repository port, state/resource ownership
- `tool`: tool definitions, registry interfaces, metadata, safety/effect contracts
- `workspace`: authorized root, protected paths, mutation synchronization

Core packages must not import adapters, features, engines, or the composition root.

### `internal/app/*`

Application ports used by inbound adapters. TUI, ACP, and headless code should
reach concrete subsystems through this layer instead of taking implementation
escape hatches.

Important application boundaries:

- `app.Conversation` hides `engine/turn` from inbound adapters.
- `app.Agents` hides the concrete `agent.Coordinator` from TUI/ACP/headless.
- `app.Projects`, `app.Providers`, and `app.UserSettings` own persistence use cases.
- `app.Sessions` owns session discovery/load/delete operations.
- `app.Models` owns provider model discovery.

Inbound adapters must not call config persistence, provider discovery, session
filesystem stores, or coordinator methods directly.

### Composition Root

`cmd/protonman/bootstrap.go` is the main assembly point. It resolves configuration,
workspace policy, checkpoints, sandboxing, skills, permissions, coordinator,
session-owned TODO state, tool registry, tool-call service, and the initial
conversation. Cross-layer wiring belongs here rather than inside domain packages.

## Runtime Assembly Flow

The primary runtime is assembled roughly as:

```text
config.Load
  -> workspace.New + checkpoint store + sandbox launcher
  -> skill discovery + permission policy
  -> agent.Coordinator
  -> session repository + session-owned todo repository
  -> builtin registry + feature tools
  -> subagent CapabilityRegistry decorator
  -> toolcall.Service
  -> provider LanguageModel
  -> app.BuildConversation / turn.Loop
  -> TUI | headless | ACP
```

## Agent Model: Dota-Style Attributes

Canonical profile identifiers are:

| Profile | TUI | Role | Mutating | Default reasoning |
| --- | --- | --- | --- | --- |
| `universal` | `UNI` | primary software engineer and orchestrator | yes | medium |
| `strength` | `STR` | substantial implementation, fixes, refactors | yes | medium |
| `agility` | `AGI` | fast bounded read-only exploration/tracing | no | medium |
| `intelligence` | `INT` | deep reasoning, architecture, difficult/high-risk engineering | yes | high |

`Universal` owns the user's goal. `Strength` builds. `Agility` moves quickly
through evidence. `Intelligence` reasons deeply.

Only `strength`, `agility`, and `intelligence` are valid delegated profiles.
`universal` is the primary/root profile and cannot be spawned as a child.

Legacy profile identifiers are no longer accepted. Only `universal`, `strength`,
`agility`, and `intelligence` are valid profile names. Persisted user/project
configuration using removed identifiers must be updated rather than normalized at
runtime.

Profile policy is centralized in `internal/feature/agent/profile_spec.go`; avoid
scattering profile-specific permissions, reasoning defaults, or descriptions.

## Subagent Runtime Semantics

Subagents are optional runtime capability, not a different root-agent type.
Universal works normally when delegation is disabled.

Configuration:

```toml
[agent]
subagents_enabled = true
reasoning_effort = "auto"

[agent.subagents.strength]
provider = "protonman"
model = "coding-model-id"
reasoning_effort = "medium"

[agent.subagents.agility]
reasoning_effort = "low" # model omitted: inherit Universal dynamically

[agent.subagents.intelligence]
provider = "anthropic"
model = "reasoning-model-id"
reasoning_effort = "high"
```

Default delegation is enabled. Effective precedence is default -> user -> trusted project.
Subagent profile tables merge field-wise by canonical profile. Provider/model must be
specified together; reasoning may be specified independently. Configured providers must
already exist in the effective provider map and satisfy their authentication requirements.

Runtime policy is loaded from user and trusted-project configuration. The TUI does not expose slash commands that mutate subagent, project, user-config, permission-mode, agent-profile, session, or reasoning policy; those capabilities remain available through their owning configuration/runtime layers.

Disabling subagents prevents **new admission only**. It never cancels running
children and does not discard retained lifecycle records.

Capability changes are linearized with `Coordinator.Spawn`: once `SetEnabled(false)`
returns, later spawn admission cannot observe the previous enabled state.

Model and reasoning selection are also bound at `Spawn` admission. A profile with no
explicit model route inherits the current Universal model; later Universal model changes
affect only future admissions. A configured profile model is immutable for that child.
Reasoning resolution is profile override -> current global override -> profile default.
`auto`/`default` is inheritance, not a separate reasoning level.

Provider/model objects are built at the application/composition boundary. The coordinator
receives provider-neutral `sdk.LanguageModel` instances and must not learn API keys, base
URLs, provider protocols, or config persistence details.

### Tool publication while disabled

`internal/adapter/out/tool/agent/CapabilityRegistry` controls what the model sees:

- enabled: publish `subagent`
- disabled with existing live/retained agents: keep `subagent` visible for lifecycle actions but reject new `action=spawn` work
- disabled with no existing agents: hide all subagent tools

Lifecycle operations are `subagent action=wait|get|list|cancel|resume`.
This preserves control over work that existed before delegation was disabled.

The execution boundary also rejects `Spawn` while disabled. Tool visibility is
not a substitute for runtime authorization because a stale model request can
outlive a capability change.

### Child ownership and nesting

Children are coordinator-owned asynchronous runs scoped by session and parent turn.
Lifecycle state is derived from versioned domain events. Durable events are appended
before lifecycle admission or transition is acknowledged, and restart recovery replays
the per-session journal before converting process-owned live states to `interrupted`.
Normal parent turns do not poll child completion. Versioned result references are published to a turn-scoped event stream, consumed with independent cursors, deduplicated by the synthesis coordinator, and delivered to the parent as ephemeral runtime context. After that context is encoded successfully, the synthesis consumer acknowledges the version exactly once with `agent_result_consumed`; this acknowledgement is presentation/telemetry state and never re-enters the result-availability stream. Delegated work is completion-blocking by default. `optional=true` marks speculative work: it remains active for safe event buffering and can be integrated if its result becomes ready, but it does not hold the parent's completion barrier and any still-live optional child is canceled when the parent commits its final response. `depends_on` forms same-turn dependency edges to already-admitted children; dependency waiting occurs before concurrency/workspace admission and does not consume queue-timeout budget. Downstream execution requires every dependency to reach `completed`.

Parent-turn termination is also the ownership boundary for asynchronous children. Runtime-context finalization runs on successful completion, failure, and cancellation; it cancels any remaining live children for that turn and releases turn-scoped synthesis cursor/deduplication state. Retained terminal results remain available through explicit inspection until normal retention pruning.

The synthesis payload carries child status, conclusion, runtime-validated findings, verification, evidence, changed targets, and bounded blockers. Child fields remain runtime evidence and do not replace parent verification.
Child final text may include a `<proton-subagent-result>` JSON envelope. The runtime accepts only evidence references matching successful child tool observations, derives changed targets and verification independently, and safely falls back to plain text when the envelope is malformed or absent.

`subagent action=wait|get|list` remain explicit lifecycle inspection capabilities and compatibility surfaces. A wait timeout never cancels a child. Explicit cancellation uses coordinator lifecycle operations. Subagent-scoped registries
remove agent and task tools, so children cannot spawn nested children or mutate the
parent's task plan.

### Workspace scheduling

Mutating profiles acquire exclusive workspace capacity. Read-only children may
run concurrently. Writer admission prevents a stream of readers from starving a
waiting writer. Preserve this fairness property when touching scheduler code.

## Prompt Architecture

System prompt composition lives in `internal/engine/prompt` and is capability-driven.
Do not maintain separate large root prompts per provider or agent mode. The managed
prompt currently uses Prompt ABI v11 and deterministic cache-aware section ordering;
`docs/system-prompt.md` is the source of truth for prompt topology and prefix-cache
invariants.

The root identity is Universal. When agent tools are published, the prompt adds
orchestration guidance. When they are absent, the prompt becomes a clean
single-agent software engineering prompt without stale delegation instructions.

Prompt sections are derived from the effective tool surface:

- task tools -> Task Coordination section
- agent tools -> Delegation Protocol
- MCP tools -> External MCP Tools trust section
- mutating workspace tools -> Editing and Verification section
- grounding profile -> Grounding Contract
- active skills -> Skills section
- repository instructions -> Project Instructions
- model profile -> provider/model guidance hints

Do not hard-code claims that a capability exists. If the model cannot call a
capability, the prompt should normally omit instructions for it.

Keep reusable prompt material before dynamic material when semantics allow. Workspace,
role, active-goal, and skill changes must not accidentally invalidate unrelated earlier
prompt bytes. Preserve deterministic section ordering and add divergence-boundary tests
when changing prompt topology.

Prefer positive semantic tool guidance. Python, Node, shell, Go, Rust, and other
runtimes are valid engineering tools when they are the appropriate operation;
do not turn prompt wording into arbitrary runtime bans.

The prompt must never treat task state, orchestration status, or agent reports as
repository evidence. Child results are context; integration and final verification
remain the primary agent's responsibility.

## Tool System

Pure tool contracts live in `internal/core/tool`. Implementations belong under
`internal/adapter/out/tool/*`; do not put concrete tools in the domain package.

The default registry requires an explicit sandbox launcher and checkpoint store.
It fails closed if either is omitted. Registration validates definitions and
precompiles input/output JSON-schema validators before atomically publishing a
batch.

Built-in workspace tools include:

- `read`
- `math`
- `grep`
- `find`
- `ls`
- `git`
- `bash`
- `edit`

Feature tools add `web`, task tools, skill activation, agent lifecycle
operations, and dynamically discovered MCP tools.

Canonical argument names for common tools are intentionally stable:

- `bash` -> `command`
- `read` -> `path`
- `edit` (`write`/`replace`) -> `file_path`
- `grep` -> `pattern`
- `web` (`action=fetch`) -> `url`

Workspace discovery is evidence-driven: use `read` only for a known artifact, `ls`
for a known directory, `find` for path discovery, and `grep` for content search. Do
not invent a filename from a package or directory name. If `read` returns
`not_found`, do not retry the same guessed path unchanged; inspect the parent with
`ls` or discover the filename with `find` first.

### Tool-call service

`internal/engine/toolcall.Service` is the application execution boundary for
every tool call. Its pipeline is approximately:

```text
validate call
-> registry lookup
-> normalize arguments
-> input-schema validation
-> derive risk/effect/scope + permission detail
-> temporary call guard
-> static permission policy / session grant / interactive mode
-> workspace mutation gate when needed
-> bounded handler execution
-> bounded read-only recovery when explicitly supported
-> output-schema validation
-> redacted lifecycle observation
```

Do not bypass this service to execute a model-originated tool.

Structured output is a contract, not decorative metadata. If a handler declares
an output schema, `StructuredOutput` must conform to it. Keep human-readable
`Output` for presentation compatibility while preserving structured data for the
model/runtime.

Tool registries may be decorated. When adding a decorator, preserve relevant
optional registry capabilities such as compiled-validator lookup and dynamic
registration/replacement instead of accidentally degrading the wrapped registry.

## Permissions and Security

Permission policy is fail-closed. Static rule precedence is:

```text
deny > ask > allow
```

Runtime permission modes include `ask`, `always-approve`, and deny behavior;
TUI plan mode adds a temporary read-only `CallGuard` rather than weakening the
shared permission policy.

Session grants are intentionally narrow. Reuse is limited to normal-risk,
read-only calls with matching normalized semantics. Do not make mutating,
uncertain, or elevated-risk operations silently reusable.

Security-sensitive invariants:

- Workspace paths remain confined to the authorized root.
- Protected paths are hidden/rejected consistently across file tools.
- File authorization and opening must not introduce TOCTOU/symlink escapes.
- Mutations create bounded pre-edit checkpoints where required.
- Shell execution is analyzed conservatively for effect and affected paths.
- `web` fetch validates resolved destinations and redirects against SSRF rules.
- Output and scan limits must be enforced while work occurs, not only afterward.
- Cancellation must terminate process trees/resources where the platform supports it.
- External schemas/descriptions/results are untrusted data.

Do not fix security problems with superficial string filters when a boundary or
ownership invariant can solve the root cause.

## Sandbox and Checkpoints

Sandbox profiles are `off`, `workspace`, `read-only`, and `strict`. Linux prefers
native Landlock/network namespace confinement when supported, with fallback
behavior owned by `internal/platform/sandbox`. macOS uses its platform launcher.

Checkpoint retention is bounded by count, bytes, and age. Preserve the newly
created checkpoint while deterministically pruning older records.

## Configuration and Runtime State

User filesystem layout is centralized in `internal/app/appdirs`:

```text
~/.protonman/
  config.toml
  sessions/
  checkpoints/
  skills/
  logs/
```

`PROTONMAN_HOME` may replace the effective user home for Protonman data.
Project-local resources live under `<workspace>/.protonman/`. User-global state lives under `~/.protonman/`; filesystem state must use the Protonman namespace exclusively.

Configuration layering is:

```text
built-in defaults
-> user config
-> trusted project config
-> session/runtime commands where applicable
```

Project config is detected but not applied until the workspace is trusted.
Do not bypass trust checks for project-local config or skills.

Selected fields carry provenance (`default`, `user`, `project`) for TUI display.
When adding a user-visible layered setting, consider whether it also needs a
provenance field, user persistence method, project persistence method, runtime
application path, TUI rendering, and tests for precedence.

TUI-local agent runtime state survives Bubble Tea program restarts. A UI restart
must not silently reset profile, reasoning effort, max tool calls, or subagent
enablement to startup config.

## Sessions and TODO Ownership

A session ID is the durable ownership boundary for conversation state and tasks:

```text
~/.protonman/sessions/<session-id>/
  state.json
  todo.md
```

`todo action=get|update` is bound to the active session repository. ACP uses
session-specific registry overlays so concurrent sessions cannot share task state.
Workspace file tools must not mutate private session task resources.

TODO mutation uses durable optimistic concurrency. Read the latest snapshot,
use its exact revision for updates, and on revision conflict refresh and reconsider
the patch. Never blindly replay stale task operations.

Task metadata is coordination state, not evidence that source code changed or a
test passed. Mark status complete only when the underlying work is complete.
Subagents cannot mutate the parent task plan.

## MCP Integration

MCP tools are outbound external capabilities under
`internal/adapter/out/tool/mcp` and are namespaced:

```text
mcp.<server>.<tool>
```

Discovery is resource-bounded and catalog registration is atomic. Server names,
tool names, manifests, schemas, and duplicate namespaces are validated before
publication.

Treat MCP safety declarations conservatively:

- an explicit mutating declaration is always honored
- a read-only declaration relaxes policy only when locally trusted
- otherwise mutability remains unspecified/potentially mutating

MCP descriptions are normalized as external metadata and cannot override system,
project, permission, safety, or user instructions.

Validate MCP input/output schemas using the same SDK contract machinery used by
built-in tools. Contract violations should retain useful diagnostics such as
server identity, catalog generation, schema fingerprint, expected JSON type, and
actual JSON type.

Do not blindly retry an external mutating call after an ambiguous transport
failure. First determine whether the remote side may already have changed state.

## Protonman SDK and Models

`proton-sdk` is provider-neutral and must not import CLI-owned `internal/*` or
`cmd/*` packages. It owns:

- `LanguageModel` and streaming interfaces
- messages, content parts, tools, tool results, and usage
- model capabilities and token limits
- reasoning effort model options
- schema compilation/validation
- provider error and rate-limit normalization
- retry decisions and bounded backoff
- middleware composition
- stream collection and history helpers

Provider protocol implementations live under `proton-sdk/provider/*`.

`internal/adapter/out/model` adapts configured providers and remote model
catalog metadata into the SDK interface. Keep provider-specific wire behavior in
the provider/adapter layer rather than contaminating the turn engine.

Model profiles can affect reasoning support, context/output limits, modalities,
tool/schema compatibility, and prompt hints. When adding provider compatibility,
prefer explicit model-profile/schema lowering over weakening the canonical tool
contract for every provider.

Streams must be closed exactly once on success and every failure/cancellation
path. Preserve the original processing error over a secondary close error when
both occur.

`internal/adapter/out/model` may add provider-specific recovery around an SDK model when the behavior cannot be expressed as a provider-neutral SDK invariant. OpenCode free-model empty-stream recovery is intentionally narrow: retry only before any visible text/tool-call output, preserve the same session identity on every attempt, cap recovery at two retries with bounded backoff and a 30-second no-output watchdog, discard metadata from abandoned attempts, and surface the final incomplete/empty response instead of replaying after visible output. User-facing diagnostics distinguish exhausted empty output (`EMPTY_RESPONSE`) from an incomplete stream (`STREAM_INCOMPLETE`).

## Turn Engine

`internal/engine/turn` owns the multi-round model/tool state machine. Inbound
adapters consume `app.Conversation`; they must not import `turn` directly.

Important turn responsibilities include:

- model request streaming
- capability-aware tool publication
- system prompt composition
- grounding requirements
- tool execution/results
- repeated-call/loop protection
- tool-result budgets
- deadline propagation
- reasoning policy
- mutation verification state
- event-driven subagent result delivery, completion barriers, and redacted synthesis-efficiency telemetry (`subagent_result_bytes`, consumed/duplicate bytes, synthesis agents, and wait-snapshot bytes)

At least one global termination bound must remain active. Do not accidentally
construct an unbounded model/tool loop by disabling both tool-count and time bounds.

## TUI Conventions

TUI is presentation logic under `internal/adapter/in/tui`. Keep domain semantics
outside it. In particular, do not let TUI directly own config persistence,
provider discovery, session storage, or concrete coordinator control.

Current important slash commands are intentionally canonical and small:

```text
/help
/model
/provider
/skills
/agents
/todo
/transcript [clear]
/call
/quit
```

Agent lifecycle presentation should aggregate by agent identity rather than dump
raw orchestration RPC noise. Active work belongs in live status/panes; terminal
results remain useful in transcript/history. Internal IDs are appropriate in the
detailed `/agents` inspection view, not as constant visual clutter.

The TUI projects lifecycle/tool activity into Dota-style presentation intents without changing domain state. Keep `queued`, `running`, `completed`, `failed`, and related lifecycle values authoritative in `internal/feature/agent`; labels such as `W8`, `Roaming`, `Farming`, `Skilling`, `Ganking`, `Pushing`, `Defending`, `Sticking`, `Integrated`, `Care`, `B`, and `Ready` belong under `internal/adapter/in/tui/state/agentui`. `Sticking` means a result is available; `Integrated` means that result version was successfully encoded into parent runtime context. Prefer deterministic signals such as profile, tool kind, and lifecycle event over guessing activity from free-form model prose.

Subagent-off is a non-default state and should be visible without permanently
spending footer space on the default enabled state. Existing children must remain
inspectable when delegation is disabled.

Shell result presentation should be generic and semantic rather than special-case
one ecosystem. Python, Node, Cargo/Rust, Make, Docker, Java/Gradle/Maven, PHP,
Ruby, .NET, Terraform, Kubernetes, and future command families should reuse the
same execution/result model where possible.

Minimal TUI presentation follows semantic density rather than blanket suppression.
Routine read/search operations stay compact, mutations retain material effects, and
denied/failed operations retain diagnostic detail. The composer metadata owns the
active model/profile/workspace/mode context; transient busy status should prefer the
active tool and target. Scrolling must preserve a single composer and semantic
transcript position while streaming updates continue. Idle footer help should expose
primary actions only; secondary shortcuts belong in contextual views or `/help`.

TUI rendering is intentionally side-effect free. `View()` and pane `Render` methods must
only read presentation snapshots; they must not resize viewports, alter scroll position,
change pane stacks, or mutate runtime/domain state. Runtime events request layout changes,
and the root Bubble Tea `Update` boundary reconciles layout once per event. Conversation
viewport state explicitly distinguishes following the live tail from reading older content,
so streaming updates preserve semantic scroll anchors. Pane rendering receives
`paneRenderContext` rather than the root model; pane key handlers return typed actions for
the root to apply instead of mutating the root model directly. Root runtime state is grouped
by ownership: agent, turn, model selection, session, project, conversation, TODO,
presentation, and execution policy. Preserve these boundaries instead of adding new flat
fields to `bubbleModel` without a clear orchestration-level reason.

When changing TUI behavior, test at the smallest useful layer:

- pure renderer/state unit tests
- Bubble Tea update/command tests
- PTY E2E tests for terminal interaction when keyboard/layout behavior matters

Avoid snapshots that assert irrelevant whitespace if semantic assertions are
more stable.

## Runtime Defaults

Canonical defaults live in `internal/base/runtimepolicy`, not duplicated literals.
Important current defaults include:

- turn tool calls: 100
- turn timeout: disabled by default (`0`); configure explicitly when a whole-turn ceiling is required
- round timeout: 5m
- tool permission timeout: 2m
- tool execution timeout: 2m
- model request timeout: 5m
- subagent max runtime: 30m
- subagent wait timeout: 30s
- subagent queue timeout: 2m
- max live subagents: 16
- max retained subagents: 64
- retained subagent result TTL: 24h

Other resource limits such as tool-result budgets, read scan bytes, checkpoint
retention, and infrastructure timeouts also belong in runtime policy or the
boundary that owns the resource. Avoid magic-number drift across packages.

## Testing and Verification

Use focused tests during development and the full suite before declaring a
cross-cutting change complete.

Common commands:

```sh
# Full repository suite, including architecture and E2E
make test

# Equivalent direct Go invocation
go test ./...

# E2E only
make test-e2e

# Full race detector
make test-race

# Format and vet
make lint

# Build binary
make build
```

For concurrency-heavy agent/tool/TUI changes, a useful targeted race run is:

```sh
go test -race \
  ./internal/feature/agent \
  ./internal/adapter/out/tool/agent \
  ./internal/adapter/in/tui
```

Run architecture tests whenever moving packages, adding cross-layer imports, or
introducing a new application boundary:

```sh
go test ./test/architecture
```

High-risk changes should usually add regression coverage at the boundary where
the bug was observable. Examples:

- provider/tool publication -> mock HTTP E2E request inspection
- config precedence -> config package + TUI/application tests
- subagent lifecycle -> coordinator tests + race detector
- terminal UX -> TUI update tests or PTY E2E
- tool schemas -> generic registry/toolcall contract tests
- MCP discovery -> catalog atomicity, contract, and concurrency tests
- filesystem security -> boundary-focused tool/workspace tests

## Change Placement Guide

When implementing a change, place it according to ownership:

| Change | Primary location |
| --- | --- |
| tool domain metadata/risk/effect contract | `internal/core/tool` |
| built-in workspace tool implementation | `internal/adapter/out/tool/builtin` |
| subagent tool adapter | `internal/adapter/out/tool/agent` |
| agent scheduling/lifecycle/profile policy | `internal/feature/agent` |
| prompt wording/composition | `internal/engine/prompt` |
| model/tool turn behavior | `internal/engine/turn` |
| permission execution pipeline | `internal/engine/toolcall` + `internal/core/permission` |
| user/project TOML persistence | `internal/adapter/out/config`, exposed via `internal/app` |
| terminal interaction/rendering | `internal/adapter/in/tui` |
| provider model discovery/adaptation | `internal/adapter/out/model` |
| provider wire protocol | `proton-sdk/provider/*` |
| session persistence | `internal/adapter/out/sessionfs` |
| reusable low-level defaults/helpers | `internal/base/*` only if truly dependency-free |
| composition/wiring | `cmd/protonman` |

## Implementation Style

Prefer idiomatic Go and explicit contracts:

- keep interfaces small and owned by the consumer when practical
- validate at boundaries before launching expensive or concurrent work
- make invalid states difficult to construct
- prefer typed errors/sentinels that callers can classify with `errors.Is/As`
- preserve context cancellation and deadlines through every layer
- bound queues, buffers, retained state, scans, and external payloads
- avoid goroutine leaks and channels that no owner can close
- keep mutex scope small, but preserve linearization where correctness needs it
- clone externally mutable slices/maps/JSON schemas at ownership boundaries
- avoid global mutable state unless it is an intentional immutable registry/default
- use comments to explain invariants and why, not to narrate obvious syntax

For refactors, maintain behavior with focused tests before broad movement. Do not
create god packages or generic utility dumping grounds to reduce file count.
Architecture and ownership are more important than making a directory look small.

## Concurrency Checklist

For asynchronous/concurrent changes, explicitly identify:

1. owner and lifetime of every goroutine/task
2. cancellation source and propagation
3. queue/admission timeout versus execution timeout
4. shared state lock/atomic ownership
5. ordering/linearization requirements
6. backpressure and retained-state bounds
7. terminal-state publication and subscriber shutdown
8. whether writer/read fairness can regress
9. what happens during shutdown and partial failure

Passing a happy-path test is not sufficient evidence for concurrency correctness.

## Git and Repository Hygiene

Before editing, inspect `git status` when existing work may be present. Preserve
unrelated modifications. Do not reset, checkout, restore, clean, or rewrite
history merely to simplify the task.

When commits are explicitly requested:

- inspect the diff first
- keep commits logically scoped and reviewable
- run relevant tests before each meaningful commit when practical
- use concise conventional messages describing the behavior changed
- do not bundle unrelated user changes

Generated binaries and temporary diagnostics do not belong in commits unless the
repository explicitly tracks them. Remove debugging artifacts introduced by the
current task.

## Documentation Rules

`README.md` is user-facing product/usage documentation.
`docs/architecture.md` explains architectural boundaries.
`AGENTS.md` is the coding-agent working contract.

Keep these roles distinct. When behavior changes, update the smallest relevant
document rather than duplicating the same prose everywhere.

Documentation must describe current behavior, not planned behavior as if already
implemented. Prefer canonical identifiers and current package paths. Removed agent
profile names should be mentioned only when documenting migration from older
configuration; current runtime behavior rejects them.

## Completion Checklist

Before declaring a task complete, verify the relevant subset of:

- requested behavior is implemented, not merely planned
- final diff is focused and contains no accidental files
- architecture boundaries still hold
- tool input/output contracts still validate
- security/trust boundaries remain fail-closed
- cancellation/deadline/resource limits still propagate correctly
- subagent enable/disable behavior remains consistent at config, registry, prompt, and execution layers
- existing delegated work remains manageable after disabling new delegation
- session/TODO ownership cannot leak across sessions
- TUI state survives expected restarts/reconfiguration
- provider-specific compatibility does not weaken canonical contracts globally
- targeted tests pass after the final mutation
- broader tests are run when blast radius warrants them
- race tests are run for meaningful concurrency changes
- no success claim depends solely on a subagent report

## Useful Starting Points

For common investigations, begin here:

- startup/wiring: `cmd/protonman/bootstrap.go`
- architecture guardrails: `test/architecture/dependency_test.go`
- tool contracts: `internal/core/tool/`
- default tools: `internal/adapter/out/tool/builtin/registry.go`
- tool execution: `internal/engine/toolcall/service.go`
- turn loop: `internal/engine/turn/`
- system prompt: `internal/engine/prompt/prompt.go`
- agent profiles: `internal/feature/agent/agent.go`, `profile_spec.go`
- agent lifecycle/concurrency: `internal/feature/agent/coordinator.go`, `lifecycle.go`, `scheduler.go`
- subagent publication: `internal/adapter/out/tool/agent/capability_registry.go`
- configuration: `internal/adapter/out/config/`
- TUI commands/state: `internal/adapter/in/tui/`
- sessions: `internal/core/session/`, `internal/adapter/out/sessionfs/`
- tasks: `internal/feature/todo/`, `internal/adapter/out/tool/todo/`
- MCP: `internal/adapter/out/tool/mcp/`
- model adaptation: `internal/adapter/out/model/`
- provider SDK: `proton-sdk/`, `proton-sdk/provider/`

When uncertain, follow evidence from these ownership points outward instead of
adding a shortcut across layers.
