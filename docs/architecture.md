# Clean Architecture Blueprint

Proton follows **Clean Architecture** (Hexagonal / Ports and Adapters) principles. The codebase maintains strict concentric dependency boundaries where dependencies point inward toward the core domain.

```
       +-------------------------------------------------------------+
       |                  cmd/proton (Composition Root)              |
       |  +-------------------------------------------------------+  |
       |  |     Inbound Adapters (Driving / Presentation)         |  |
       |  |  internal/tui  |  internal/acp  |  internal/headless  |  |
       |  |  +-------------------------------------------------+  |  |
       |  |  |         Application Layer (internal/app)        |  |  |
       |  |  |  conversation.go  agents.go   models.go        |  |  |
       |  |  |  providers.go     projects.go sessions.go      |  |  |
       |  |  |  +-------------------------------------------+  |  |  |
       |  |  |  |          Core Domain Entities             |  |  |  |
       |  |  |  |   modelprofile/  permission/   session/   |  |  |  |
       |  |  |  |   tool/          workspace/               |  |  |  |
       |  |  |  +-------------------------------------------+  |  |  |
       |  |  +-------------------------------------------------+  |  |
       |  |     Outbound Adapters (Driven / Infrastructure)       |  |
       |  |  internal/adapter/sessionfs                           |  |
       |  |  internal/adapter/tool/{agent,builtin,mcp,skill,todo,web}|
       |  |  internal/model                                       |  |
       |  |  internal/config                                      |  |
       |  |  internal/turn (Orchestration Engine)                 |  |
       |  |  +-------------------------------------------------+  |  |
       |  |  |          Foundational Group (internal/base)     |  |  |
       |  |  |  buildinfo/ contextutil/ envconfig/ failure/    |  |  |
       |  |  |  glob/      runtimepolicy/                      |  |  |
       |  |  +-------------------------------------------------+  |  |
       |  +-------------------------------------------------------+  |
       +-------------------------------------------------------------+
```

---

## 1. Composition Root (`cmd/proton/`)
The single assembly point of the application:
- `main.go`: Process entry point, signal trapping, and presentation mode selection (`tui`, `acp`, `headless`, `session`).
- `bootstrap.go`: Instantiates infrastructure stores, domain policies, and wires outbound adapters into application services (`app.BuildConversation`, `app.NewSessions`, `app.NewAgents`).
- `headless_mode.go`: Headless CLI dispatch consuming `app.Conversation`.
- `session_commands.go`: CLI subcommands for session inspection, resume, and cleanup.

---

## 2. Application Layer (`internal/app/`)
Defines the primary application use cases and boundaries for inbound driving adapters:
- `conversation.go`: `Conversation` interface port (`Run(ctx, messages, sink) (Result, error)`) and `BuildConversation` factory.
- `agents.go`: Subagent lifecycle management, subscription, and cancellation use cases.
- `models.go`: Remote provider model discovery use cases.
- `providers.go`: User provider settings mutations and persistence use cases.
- `projects.go`: Project-local configuration mutations and project discovery / initialization.
- `sessions.go`: Persisted session loading, listing, and deletion use cases.
- `appdirs/`: Filesystem layout resolution (`.proton/`, `config.toml`, `sessions/`, etc.).

*Rule*: Inbound adapters interact exclusively through `internal/app` and never touch concrete turn loops, config persistence, or direct database/filesystem stores.

---

## 3. Core Domain Entities (`internal/*`)
Pure business rules and domain definitions. No `domain-ish` parent folder is created to maintain idiomatic, flat Go packaging:
- `internal/modelprofile/`: Model capability schemas, token limit calculations, reasoning profile definitions.
- `internal/permission/`: Security modes (`ask`, `always-approve`, `deny`), path permission rules, evaluation policies.
- `internal/session/`: Session entities, state models, message conversions, and repository port `session.Repository`.
- `internal/tool/`: Pure domain contracts for tools: `Handler` interface, `Registry`, `Specification`, parameter metadata, call context. Contains zero tool implementations.
- `internal/workspace/`: Filesystem root isolation, directory safety gates, mutation boundaries.

*Rule*: Core domain packages never import outer layers (`cmd/proton`, `app`, `turn`, `tui`, `acp`, `headless`, or adapters).

---

## 4. Interface Adapters (`internal/adapter/`, `internal/model/`, `internal/config/`, `internal/turn/`)

### Inbound (Driving) Presentation Adapters
- `internal/tui/`: Presentation-only terminal user interface built with Bubble Tea. Modularized across view components, slash commands, and history cell formatters.
- `internal/acp/`: Agent Client Protocol (ACP) JSON-RPC 2.0 protocol adapter.
- `internal/headless/`: Non-interactive output adapter for CI/CD and scripts (text/NDJSON stream).

### Outbound (Driven) Infrastructure Adapters
- `internal/adapter/sessionfs/`: File-backed storage implementation of `session.Repository`.
- `internal/adapter/tool/`: Unified home for **all tool implementations** satisfying `tool.Handler`:
  - `agent/`: Subagent orchestration tools (`delegate_task`, `wait_agent`, etc.).
  - `builtin/`: Core developer tools (`read_file`, `write_file`, `edit_file`, `bash`, `glob`, `grep`, `list_dir`).
  - `mcp/`: External Model Context Protocol server discovery and tool registration.
  - `skill/`: Agent skill activation (`activate_skill`).
  - `todo/`: Work tracking tools (`get_todo`, `update_todo`).
  - `web/`: Network web fetching with sandbox isolation (`web_fetch`).
- `internal/model/`: Provider integration and SDK translation:
  - `provider_preset.go`: Endpoint and protocol presets.
  - `provider_discovery.go`: Dynamic model discovery over provider APIs.
  - `provider_catalog.go`: Provider model catalog normalization.
  - `model_profile.go`: Profile resolution and SDK model type aliases.
  - `client_factory.go`: Client instantiation and options (`ClientOption`).
  - `sdk_adapter.go`: `proton-sdk` provider model adapter with capability overrides.
- `internal/config/`: TOML configuration loading, merging (user/project), and persistence (`config.go`, `document.go`, `merge.go`, `load.go`, `user_save.go`, `project_save.go`).
- `internal/turn/`: Turn orchestration engine and loop state machine driving model streaming, tool execution, grounding, and verification.

---

## 5. Foundational Grouping (`internal/base/`)
Pure leaf packages with **zero dependencies on any other internal package**:
- `internal/base/buildinfo`: Application version and User-Agent construction.
- `internal/base/contextutil`: Detached timeout context helper (`DetachedTimeout`).
- `internal/base/envconfig`: Environment variable names (`PROTON_*`) and boolean parser.
- `internal/base/failure`: Domain error classification codes and failure traits.
- `internal/base/glob`: Pure in-memory string wildcard matching (`*`, `?`).
- `internal/base/runtimepolicy`: Global runtime defaults, timeout durations, and buffer limits.

---

## 6. Architectural Enforcement
Architecture boundaries are permanently enforced by automated tests in `internal/architecture/dependency_test.go`:
1. Core packages do not depend on outer layers.
2. Base packages (`internal/base/*`) have zero internal dependencies.
3. Inbound adapters (`tui`, `acp`, `headless`) depend on `app.Conversation`, never on `turn`.
4. Inbound adapters do not perform config persistence or provider discovery directly.
5. TUI does not depend directly on session persistence or project discovery.
6. All tool implementations reside exclusively in `internal/adapter/tool/`.
7. `internal/app` and `internal/model` file sets conform strictly to the architecture blueprint.
8. The `proton-sdk` has zero dependencies on internal CLI packages.
