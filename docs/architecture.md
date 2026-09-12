# Clean Architecture Blueprint

Protonman follows Clean Architecture / Ports and Adapters. Dependencies point toward
stable application and domain contracts; concrete protocol, persistence, provider,
terminal, and operating-system details remain at the edges.

```text
cmd/protonman/                    composition root and mode selection
        |
        +--> internal/adapter/in/           driving adapters
        |      acp/                          ACP JSON-RPC over stdio
        |      headless/                     non-interactive CLI
        |      tui/                          Bubble Tea TUI
        |
        +--> internal/app/                  application use-case ports
        |
        +--> internal/engine/               orchestration
        |      prompt/                       capability-driven system prompt
        |      toolcall/                     tool execution boundary
        |      turn/                         model/tool state machine
        |
        +--> internal/core/                 domain contracts and policies
        |      conversation/                 retention/history policy
        |      modelprofile/                 model capability policy
        |      permission/                   authorization policy
        |      session/                      session aggregate and repository port
        |      tool/                         tool contracts and metadata
        |      workspace/                    workspace safety and mutation policy
        |
        +--> internal/feature/              cohesive domain features
        |      agent/ project/ skill/ todo/
        |
        +--> internal/adapter/out/          driven adapters
        |      config/ model/ sessionfs/ tool/
        |
        +--> internal/platform/             OS/runtime infrastructure
               checkpoint/ sandbox/ telemetry/

internal/base/                    dependency-free internal leaf utilities
proton-sdk/                       provider-neutral model SDK
```

## 1. Composition Root (`cmd/protonman/`)

`cmd/protonman` is the application assembly boundary. It selects TUI, headless, ACP,
and session-oriented command modes, resolves effective configuration/workspace policy,
and wires concrete adapters into application and engine contracts.

`bootstrap.go` is the primary assembly point. Cross-layer construction belongs here,
not inside the core domain or inbound presentation packages.

## 2. Inbound Adapters (`internal/adapter/in/`)

Driving adapters translate external interaction into application operations:

- `acp/`: ACP JSON-RPC/stdin-stdout protocol handling.
- `headless/`: non-interactive CLI output for scripts and CI.
- `tui/`: Bubble Tea terminal presentation and interaction.

Inbound adapters consume application ports such as `app.Conversation`, `app.Agents`,
`app.Models`, `app.Projects`, `app.Providers`, and `app.Sessions`. They must not bypass
those boundaries to persist configuration, discover provider models, access session
filesystem stores, or control concrete coordinator/turn implementations directly.

The TUI root model is an event orchestrator. Bubble Tea `Update` owns state transitions and
layout reconciliation; `View` is pure rendering. Runtime state is grouped into agent, turn,
model-selection, session, project, conversation, TODO, presentation, and execution-policy
ownership blocks rather than accumulated as unrelated flat fields. Pane renderers consume
immutable presentation snapshots, and pane interactions return typed actions for the root
to apply instead of mutating it directly. Transcript cells use one width-aware render
contract so viewport width remains the rendering source of truth.

The `internal/adapter/in/tui/runtime` root is intentionally a thin orchestration shell.
Pure catalog/filtering/projection/policy logic belongs in focused runtime subpackages or in
the owning `view/*` package. Current examples include `modelcatalog`, `modelpicker`,
`modelsetup`, `provider`, `permissionpolicy`, `permissionbridge`, `reasoningpolicy`, `projectconfig`, `keyboardpolicy`, `paneutil`, `transientnotice`, `transcriptutil`, `cmdpolicy`, and `conversation`.
A runtime subpackage must not import the root `runtime` package; dependencies flow from the
root shell into focused helpers, never back upward. This keeps Bubble Tea wiring from
becoming a package-wide dependency magnet.

The runtime-root production-file count is guarded as a ratchet in the architecture tests.
The refactor that introduced this rule reduced the root from 68 production files to 60,
and subsequent ownership extraction reduced the ratchet to 32. The budget is a regression
guard, not a target architecture: do not raise it to accommodate new behavior. Prefer extracting cohesive ownership or consolidating an existing shell.

## 3. Application Layer (`internal/app/`)

The application layer exposes use cases needed by inbound adapters and hides concrete
engine/infrastructure implementations.

Important boundaries include:

- `Conversation`: provider-neutral conversation execution port.
- `Agents`: subagent lifecycle/application operations.
- `Models`: remote provider model discovery.
- `Providers`: user provider configuration mutations and persistence.
- `Projects`: project discovery, trust, and project settings.
- `Sessions`: persisted session discovery/load/delete operations.
- `appdirs/`: canonical Protonman filesystem namespace resolution.

## 4. Engine (`internal/engine/`)

The engine is orchestration, not an outbound adapter:

- `prompt/` composes capability-driven system prompts using deterministic, cache-aware ordered sections. The managed prompt currently uses Prompt ABI v12; see [`system-prompt.md`](system-prompt.md) for ordering and prefix-cache invariants.
- `toolcall/` validates and authorizes model-originated tool calls before execution.
- `turn/` owns the bounded multi-round model/tool state machine, streaming, grounding,
  tool-result budgets, repeated-call protection, reasoning policy, verification state, and ephemeral event-driven runtime context delivery. Runtime-context finalization is a terminal-turn invariant: it runs on success, failure, and cancellation so turn-owned asynchronous work and consumer state cannot outlive their parent.

Inbound adapters must use `app.Conversation` rather than importing `engine/turn` directly.

## 5. Core Domain (`internal/core/`)

Core packages define stable policies and contracts and do not import adapters, features,
engines, or the composition root.

- `conversation/`: provider-neutral retention, compaction, and historical tool-message policy.
- `modelprofile/`: model metadata, limits, modalities, reasoning and schema compatibility.
- `permission/`: permission modes, rules, grants, and evaluation.
- `session/`: session identity, aggregate resources, revisions, state, and repository port.
- `tool/`: tool definitions, registry contracts, calls/results, risk/effect/mutability metadata.
- `workspace/`: authorized root, protected paths, mutation synchronization and path policy.

## 6. Features (`internal/feature/`)

Feature packages own cohesive product behavior built on core contracts:

- `agent/`: canonical profiles, dependency-aware scheduling, lifecycle/events, delegation policy, required-vs-optional completion barriers, and versioned result delivery/consumption acknowledgement. Result availability drives the synthesis stream; the separate `agent_result_consumed` event is emitted only after successful parent-context encoding and never feeds back into that stream. Dependency edges reference already-admitted children in the same parent turn, so the runtime forms an acyclic execution graph by construction.
- `project/`: project discovery/trust behavior.
- `skill/`: skill discovery and activation domain behavior.
- `todo/`: session task-plan state and optimistic concurrency.

Feature packages are not presentation or persistence dumping grounds. Concrete filesystem,
network, provider, and terminal concerns remain in adapters/platform packages.

## 7. Outbound Adapters (`internal/adapter/out/`)

Driven adapters implement infrastructure-facing ports:

- `config/`: layered TOML loading, merge, provenance, and persistence. Effective settings are created through `DefaultSnapshot()`, whose product defaults come only from `internal/base/runtimepolicy`; user and project writers share one atomic document persistence primitive while retaining scope-specific security checks and file modes. Persisted TOML provider/model records use dedicated file-schema structs and explicit conversion into runtime config types, so runtime representation changes do not silently redefine the on-disk format. The current contract and refactor boundaries are documented in [`settings.md`](settings.md).
- `model/`: provider presets, discovery, catalog normalization, SDK adaptation, and narrowly scoped provider-specific wrappers. Models can use a shared provider+endpoint+model low-concurrency scheduler here for bounded admission, low concurrency, adaptive pacing, and route-wide provider cooldowns. The default `auto` policy currently recommends it for OpenCode free models; `/low on` can force the same provider-neutral scheduler for any active provider/model and `/low off` bypasses it. Replay-safe OpenCode free-model stream recovery remains a separate wrapper.
- `sessionfs/`: file-backed session repository and agent lifecycle persistence.
- `tool/agent/`: subagent lifecycle tool and capability publication.
- `tool/builtin/`: workspace coding tools (`read`, `math`, `grep`, `find`, `ls`, `git`, `bash`, `edit`).
- `tool/mcp/`: external MCP discovery, validation, and registration.
- `tool/skill/`: skill activation tool.
- `tool/todo/`: session-bound task tool.
- `tool/web/`: network web capability.

All concrete tool handlers live under `internal/adapter/out/tool/*`; pure tool contracts
remain in `internal/core/tool`.

## 8. Platform (`internal/platform/`)

Platform packages own operating-system/runtime infrastructure that is neither domain logic
nor a protocol adapter:

- `checkpoint/`: bounded pre-mutation checkpoint persistence.
- `sandbox/`: OS-specific confinement and process launch behavior.
- `telemetry/`: runtime telemetry infrastructure. Subagent orchestration reports redacted result-size, consumed-size, duplicate-suppression, synthesis-batch, and diagnostic wait-snapshot measurements without task or model-output content.

## 9. Foundational Utilities (`internal/base/`)

`internal/base/*` packages are internal leaves with zero dependencies on other internal
or `cmd/*` packages. Current responsibilities include:

- `analysis/`
- `buildinfo/`
- `contextutil/`
- `envconfig/`
- `failure/`
- `glob/`
- `pathutil/`
- `runtimepolicy/`
- `strutil/`

Only truly dependency-free reusable policy/helpers belong here.

## 10. Session Aggregate Ownership

A session ID is the durable ownership boundary for conversation state, the active goal,
and task state. At minimum, session resources include:

```text
~/.protonman/sessions/<session-id>/
  state.json   # conversation/session controls, including ActiveGoal
  todo.md      # durable task-plan state
```

The composition root restores `ActiveGoal` into the initial `app.Conversation`, and the TUI
preserves it across Bubble Tea program restarts before rebuilding the runner. Setting a goal
through `/goal <detail>` is an execution entry point: it updates the managed conversation goal
and immediately starts a normal model/tool turn with the goal text as the user objective.
Inspecting or clearing the goal does not start a turn.

The current TODO repository remains session-scoped rather than goal-scoped. Goal-to-plan
binding, plan supersession, and goal-completion gating are therefore not architectural
invariants yet and must not be assumed by adapters or documentation.

`sessionfs` may also maintain session-owned agent lifecycle projection/journal resources.
Their filenames are persistence details, but their ownership is not: concurrent sessions
must never share TODO or lifecycle state.

TUI/headless bind task tools to the active session. ACP creates session-specific registry
overlays. Workspace file tools cannot mutate private session resources.

User-global state lives under `~/.protonman/`; trusted project-local state lives under
`<workspace>/.protonman/`. Namespace resolution is centralized in `internal/app/appdirs`.

## 11. Architectural Enforcement

`test/architecture/dependency_test.go` enforces the dependency rules. Do not weaken the
guards to make an architectural violation pass.

Key invariants include:

1. Core packages do not depend on outer layers.
2. Base packages have zero internal dependencies.
3. Inbound adapters use application ports rather than concrete turn/config/session/model implementations.
4. Tool implementations live under `internal/adapter/out/tool/`.
5. `proton-sdk` has zero dependencies on CLI-owned `internal/*` or `cmd/*` packages.
6. Composition/wiring remains in `cmd/protonman` rather than leaking into domain packages.
7. The TUI runtime root stays within its ratcheting production-file budget.
8. Focused `tui/runtime/*` subpackages never import the root `tui/runtime` package.

Run `go test ./test/architecture` whenever moving packages or changing dependency direction.

## 12. Safety and Resource Invariants

Resource and security limits are enforced at the owning boundary, while work occurs:

- `web` validates resolved destinations and redirects against SSRF policy.
- model streams are closed exactly once across success, error, and cancellation paths.
- subagent live/retained state and result retention remain bounded.
- session grants are reusable only for matching normal-risk read-only semantics.
- checkpoints are bounded by count, bytes, and age.
- `read`, `find`, and `git` enforce scan/output/process limits before unbounded buffering.
- missing `read` targets recover by inspecting the nearest existing readable ancestor; discovery evidence is returned as structured `ls` entries as well as compatibility text so the model and TUI share one recovery source of truth.
- repeated identical terminal read failures are suppressed by the turn progress guard until a successful mutation advances the workspace epoch.
- workspace file authorization and opening must not introduce symlink/TOCTOU escapes.
- model-originated tools execute through `internal/engine/toolcall.Service`.

These invariants are architecture, not presentation details. UI, pagination, provider, or
sandbox refactors must preserve them.
