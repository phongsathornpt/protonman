# Protonman

Protonman is an autonomous, high-performance Go-based AI coding agent designed with clean architecture, strict security boundaries, fail-closed permission policies, and extensible agent skills.

Protonman operates across multiple execution environments:
- **Interactive TUI**: A terminal interface built with Bubble Tea, featuring live streaming, markdown formatting, collapsible task panes, and modal approval controls.
- **Headless CLI**: A scriptable runner supporting one-shot prompts, piped input via stdin, and structured text or JSON output.
- **ACP Server**: An Agent Client Protocol server serving line-delimited JSON-RPC over stdio for IDE and editor integrations.

---

## Architecture Overview

Protonman isolates external effects behind strict application boundaries. External commands and file modifications must pass policy checks and pre-edit checkpointing before execution.

```text
┌────────────────────────────────────────────────────────────────────────┐
│                        User Interfaces / Adapters                      │
│      Bubble Tea TUI    │    Headless Runner    │       ACP Server      │
└───────────────┬────────────────────────┬───────────────────────┬───────┘
                │                        │                       │
                ▼                        ▼                       ▼
┌────────────────────────────────────────────────────────────────────────┐
│                         Application Turn Loop                          │
│     - Multi-round autonomous turn management & deadline propagation    │
│     - Model client integration (OpenCode, Protonman, Ollama, OpenAI)   │
│     - Subagent coordination & task delegation                          │
│     - Agent Skills progressive disclosure                              │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                       Tool-Call Service & Policy                       │
│     - Mode evaluation: ask | plan | always-approve                     │
│     - 3-tier precedence: deny > ask > allow                            │
│     - Session-scoped temporary grants                                  │
│     - Privacy-preserving redacted telemetry                            │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                      Tool Registry & Capabilities                      │
│     - Workspace boundary confinement & symlink escape prevention       │
│     - Automatic pre-edit checkpoints & rollback store                  │
│     - OS sandbox confinement (macOS Seatbelt / Linux native Landlock) │
│     - Builtin tools: read, write, patch, grep, list, bash, fetch, etc. │
└────────────────────────────────────────────────────────────────────────┘
```

See [`docs/architecture.md`](docs/architecture.md) for the package responsibility map and refactoring boundaries.

---

## Quick Start

### Install the CLI

For public releases on Linux or macOS:

```sh
curl -fsSL https://github.com/phongsathornpt/protonman/releases/latest/download/install.sh | sh
protonman --version
```

The installer detects OS/architecture, verifies the release SHA-256 checksum, and installs to `~/.local/bin` by default. Use `--version` or `--bin-dir` for an exact release or custom destination. When the repository is private, run `install.sh` from an authenticated checkout and set `GITHUB_TOKEN` or `GH_TOKEN` so private release assets can be fetched. See [`docs/install.md`](docs/install.md).

### Build from Source

Development requires Go 1.27+ and Git. Linux sandboxing uses native Landlock and network namespaces when supported; `bwrap` is an optional fallback.

```sh
# Start the interactive fullscreen TUI (default)
make tui

# Or run directly with Go
go run ./cmd/protonman

# Build the standalone binary
make build
make install   # build HEAD and install to ~/.local/bin/protonman
./bin/protonman --version
```

### Headless & Scripting

```sh
# Run a single prompt and exit
protonman -p "Explain the project architecture"

# Run non-interactively with auto-approval (always-approve mode)
protonman -y -p "Run the test suite and fix any failing tests"

# Read prompt from stdin and output JSON
cat prompt.txt | protonman --headless --output json

# Serve Agent Client Protocol (ACP) over stdio
protonman --acp

# Run within a strict OS sandbox profile
protonman --sandbox strict -p "Analyze dependencies"
```

---

## Terminal User Interface (TUI)

The fullscreen TUI is built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

### Live Interface Layout

```text
┌────────────────────────────────────────────────────────────────────────┐
│  Protonman ── Go Coding Agent                               [mode: ask]  │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  > Analyze repository structure and test coverage                      │
│                                                                        │
│  ● Assistant response streaming with sanitized ANSI formatting...      │
│                                                                        │
│  ⚙ Tool Call: read (internal/config/config.go)          [SUCCESS] │
│                                                                        │
│  ┌─ [Ctrl+O] Tasks Checklist ───────────────────────────────────────┐  │
│  │ [x] 1. Inspect configuration package                             │  │
│  │ [ ] 2. Verify subagent coordination                              │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                        │
├────────────────────────────────────────────────────────────────────────┤
│ ⚠️  Permission Request: bash "go test ./..."                           │
│    [1] (y) Allow Once                                                  │
│    [2] (s) Allow for Session                                           │
│    [3] (n) Deny                                                        │
├────────────────────────────────────────────────────────────────────────┤
│ > Type a message or '/' for commands...                                │
├────────────────────────────────────────────────────────────────────────┤
│ [Enter] send  [Shift+Tab] permission  [^P] setup  [^S] skills  [^C] quit   │
└────────────────────────────────────────────────────────────────────────┘
```

### Keybindings

| Key | Action |
| :--- | :--- |
| `Enter` | Submit current prompt / execute command |
| `?` | Open the compact shortcut reference when the composer is empty |
| `Shift+Tab` | Cycle permission mode (`ask` → `plan` → `always-approve`) |
| `Ctrl+P` | Toggle unified Model Setup (provider, model, thinking) |
| `Ctrl+S` | Toggle Agent Skills browser pane |
| `Ctrl+O` | Toggle Tasks / TODO checklist pane |
| `Ctrl+T` | Open transcript overlay (toggle raw view with `r`) |
| `PgUp` / `PgDn` | Scroll transcript viewport up/down |
| `Esc` | Park active permission modal or dismiss overlays |
| `Ctrl+C` | Cancel active operation or exit |

### In-TUI Slash Commands

Type `/` at the prompt to trigger autocomplete, or prefix a canonical command with a colon (for example `:help`):

| Command | Description | Example |
| :--- | :--- | :--- |
| `/help` | Display available commands | `/help` |
| `/permission` | Select permission mode | `/permission` |
| `/model [name]` | Open unified Model Setup or switch active model | `/model glm-5.3-flash` |
| `/provider [cmd]` | Manage and configure AI model providers | `/provider list`, `/provider opencode` |
| `/skills [name|active|toggle]` | Browse, activate, or toggle Agent Skills | `/skills pdf-processing` |
| `/agents` | Inspect live and retained subagents | `/agents` |
| `/todo [show|hide]` | Show or hide the task-plan pane | `/todo` |
| `/transcript [clear]` | Open or clear the transcript | `/transcript clear` |
| `/call <tool> <json>` | Directly execute a tool with JSON arguments | `/call read {"path":"README.md"}` |
| `/quit` | Exit Protonman cleanly | `/quit` |
| `!<command>` | Execute a shell command directly through the `bash` tool | `!git status` |

Model Setup combines provider, model, and thinking selection in one interaction. Use `↑/↓` to move through models, `←/→` to adjust thinking, `tab` to switch provider, and `enter` to apply the selection. Model catalogs stay scoped per provider; stale or missing catalogs refresh automatically and stale async results are ignored. Press `/` inside Model Setup to filter by model ID, name, vendor, or feature; `r` forces a refresh. Provider credentials remain managed through `/provider`. Direct `/model <id>` still permits custom or unlisted IDs and marks them as unverified instead of rejecting them.

Live subagent activity uses a compact Dota-style vocabulary in the status row and `/agents` view. These labels are presentation only; runtime lifecycle state remains `queued`, `running`, `completed`, and related domain states. Current mappings include `W8` for queued work, `Roaming` for AGI exploration, `Farming` for evidence gathering, `Skilling` for INT reasoning, `Ganking` for focused search, `Pushing` for implementation, `Defending` for verification, `Sticking` when a result becomes available for integration, and `Integrated` after that versioned result is delivered into the parent runtime context. `Care` marks failed/interrupted work and `B` marks cancellation/retreat.

---

## Security & Policy Engine

Protonman enforces security policy boundaries before any tool executes.

### Permission Modes

- **`ask` (Default)**: Prompts interactively whenever a tool action is not explicitly pre-approved by configuration rules.
- **`plan`**: Read-only mode. Mutating `edit` actions and non-whitelisted shell commands are blocked.
- **`always-approve`**: Automatically approves tool calls that would otherwise prompt. Explicit `deny` rules remain strictly enforced.

### Policy Evaluation

Permission rules follow strict precedence:
$$\text{deny} > \text{ask} > \text{allow}$$

An omitted rule action defaults to `deny` (fail-closed).

When prompted in `ask` mode:
- **Allow Once (`y` / `1`)**: Authorizes only this single tool call.
- **Allow for Session (`s` / `2`)**: Grants authorization for matching calls for the remainder of the session.
- **Deny (`n` / `3`)**: Rejects execution, returning a permission-denied error to the agent turn loop.

### Workspace Confinement & Checkpoints

- **Path Traversal Protection**: File operations are confined to the workspace root directory. Relative escapes (`../`) and symlink traversal outside the workspace boundary are rejected.
- **Protected Paths**: Configured protected paths (e.g. `.env`, `secrets/`, `*.pem`) are shielded from model reads, listings, and modifications.
- **Automatic Checkpoints**: Mutating file operations create pre-edit snapshots stored under `~/.protonman/checkpoints/`. File state can be restored via `edit` with `action=restore`.

### OS Sandbox Profiles

Protonman can confine sub-processes via OS-level sandboxing:
- **macOS**: Evaluates seatbelt confinement profiles via `sandbox-exec`.
- **Linux**: Uses native Landlock for filesystem confinement and a user/network namespace for blocked-network profiles when supported. Bubblewrap (`bwrap`) is retained only as a fallback for hosts missing native prerequisites. The CLI probes Landlock and user-namespace capabilities before backend selection.

| Profile | Workspace Files | Host Filesystem | Network Access |
| :--- | :--- | :--- | :--- |
| `off` | Unrestricted | Unrestricted | Allowed |
| `workspace` | Read-Write | Blocked / Temp only | Allowed |
| `read-only` | Read-Only | Blocked / Temp only | Blocked |
| `strict` | Read-Write | Blocked | Blocked |

Configure the sandbox globally via config or per-run:
```sh
protonman --sandbox strict -p "Analyze local files"
```

### Tool Deadlines

Permission resolution and approved tool execution have separate two-minute
deadlines by default. A shorter parent turn or tool context still wins. Bash
commands receive context cancellation, preserve partial output, and terminate
their process tree where the platform supports it. Set `PROTONMAN_DEBUG_LOG` to
`stderr` or a file path to inspect timeout cause, error type, and process
termination diagnostics without logging command contents.

---

## Model Providers & Catalog

Protonman routes agent model calls through `proton-sdk`, with native OpenAI-compatible and Anthropic Messages protocol support. Custom gateways and local inference remain supported through provider configuration:

### Built-in Provider Presets

| Provider | Base URL | Auth Required | Description |
| :--- | :--- | :--- | :--- |
| **OpenCode** | `https://opencode.ai/zen/v1` | No (Free) | Free-tier models with zero API key required; free-model streams use bounded recovery when the provider returns no visible output |
| **Protonman** | `https://protonman.dev/api/v1` | Yes (`plk_...`) | High-speed AI model gateway |
| **Ollama** | `http://localhost:11434/v1` | No | Local LLM inference |
| **OpenAI** | `https://api.openai.com/v1` | Yes (`sk-...`) | OpenAI-compatible API through `proton-sdk` |
| **Anthropic** | `https://api.anthropic.com` | Yes | Anthropic Messages API through `proton-sdk` |

### Available Models (Protonman Gateway)

- `deepseek-v4-flash-vision-exp` (Default, 1M context, tool-calling & vision)
- `glm-5.3-flash` (1M context, tool-calling & vision)
- `Qwen3.8-Flash` (1M context, text & vision)
- `muse-spark-1.3-contributor` (1M context, tool-calling)
- `MiniMax-M3` (1M context)

Configure providers directly inside the TUI with `/provider` or via `~/.protonman/config.toml`.

`proton-sdk` owns provider-neutral agent messages, tools, streaming events, usage/finish metadata, model registry, middleware, and provider wire adapters. The Protonman CLI keeps permission policy, tool execution, sessions, and turn orchestration outside the SDK. See [`docs/proton-sdk.md`](docs/proton-sdk.md) for the agent-first SDK contract and provider extension boundaries.

For OpenCode free models, Protonman retries a stream only when no visible text or tool call has been emitted yet. Recovery is bounded to two retries with backoff and a 30-second no-output watchdog per attempt; once visible output has started, an incomplete stream is surfaced instead of replayed to avoid duplicate output or tool calls. The TUI exposes exhausted empty-response recovery as `EMPTY_RESPONSE` and an abruptly terminated provider stream as `STREAM_INCOMPLETE`; both are presented as retryable provider failures.

---

## Agent Capabilities & Tools

Protonman uses Dota-style engineering attributes as a single agent vocabulary:

| Attribute | TUI | Role |
| :--- | :---: | :--- |
| `universal` | `UNI` | Primary software engineering agent and orchestrator; owns integration and verification |
| `strength` | `STR` | Substantial implementation, fixes, refactors, migrations, and concrete execution |
| `agility` | `AGI` | Fast read-only exploration, tracing, and focused investigation |
| `intelligence` | `INT` | Deep reasoning, architecture, difficult debugging, concurrency, performance, and high-risk engineering |

`Universal` is the root identity even when subagents are disabled. `subagent action=spawn` accepts only `strength`, `agility`, or `intelligence`; legacy CLI/config/session profile names (`pow`, `int`, `dex`, `worker`, `explorer`, `reviewer`) are normalized for compatibility but are not published in the new tool schema.

Protonman registers a suite of workspace-safe tools:

| Tool | Category | Description |
| :--- | :--- | :--- |
| `read` | File System | Read UTF-8 workspace files with byte pagination or bounded 1-based line ranges/line numbers, plus snapshot-bound byte continuations |
| `edit` | File System | Workspace edits via `write`, `replace`, `patch`, and `restore` actions with existing checkpoint safeguards |
| `grep` | Search | Regex search with include globs plus snapshot-bound cursor pagination that resumes from the prior match location |
| `find` | Search | Recursive workspace path discovery by glob with type/depth filters and snapshot-bound pagination |
| `ls` | Search | List visible directory entries with protected-path filtering and snapshot-bound pagination |
| `git` | Version Control | Git capability; `action=status` inspects working tree state |
| `bash` | Execution | Run bounded shell commands with workspace-relative `cwd`, optional `timeout_seconds`, effect analysis, and structured stdout/stderr |
| `web` | Network | Search the web with `action=search` or fetch a known URL with `action=fetch` under sandbox network policy |
| `skill` | Skills | Dynamically load an Agent Skill's full context into the session |
| `todo action=get` | Tasks | Read the current session-owned task snapshot, durable revision, and session identity |
| `todo action=update` | Tasks | Atomically patch session-owned task state using `expected_revision` from `todo action=get`; stale cross-process updates are rejected |
| `subagent action=spawn` | Multi-Agent | Spawn a persistent background subagent and return its `agent_id` immediately |
| `subagent action=wait` | Multi-Agent | Diagnostic lifecycle wait; normal child results are delivered automatically |
| `subagent action=get` | Multi-Agent | Diagnose one retained subagent and inspect its terminal result |
| `subagent action=list` | Multi-Agent | Inspect queued, running, and retained terminal subagents |
| `subagent action=cancel` | Multi-Agent | Explicitly cancel a queued or running subagent |

Session state and task plans are private user data, not workspace files. Each session owns an aggregate under `~/.protonman/sessions/<session-id>/`. When `PROTONMAN_HOME` overrides the effective home directory, the same `.protonman/sessions/<session-id>/` layout is created beneath that home:

```text
.protonman/sessions/<session-id>/
  state.json
  todo.md
```

`state.json` and `todo.md` both use durable revisions. Session saves and task patches reject stale writers instead of silently accepting last-writer-wins updates. Workspace `TODO.md` files are never used as Protonman's internal task store. Legacy flat session JSON files remain readable and migrate to the aggregate layout on the next successful save.

---

## Agent Skills

Protonman implements the open [Agent Skills Specification](https://agentskills.io). Skills are self-contained directory packages containing a `SKILL.md` (YAML frontmatter + Markdown instructions) and optional helper scripts and references.

### Discovery Locations
- **User-level**: `~/.protonman/skills/` and `~/.agents/skills/`
- **Project-level**: `<workspace>/.protonman/skills/` and `<workspace>/.agents/skills/`

> [!NOTE]
> Project-local skills and configuration are only loaded when `PROTONMAN_TRUST_PROJECT=1` is enabled. Untrusted project skills are safely skipped with a diagnostic warning.

User-global state lives under `~/.protonman/` and project-local state under `<workspace>/.protonman/`. New and existing runtime data use this namespace exclusively.

### Progressive Disclosure
1. **Catalog (Tier 1)**: Available skills are summarized as `<available_skills>` in the system prompt (~50-100 tokens per skill).
2. **Activation (Tier 2)**: When a task matches a skill, the model invokes `skill`, loading full instructions, scripts, and asset references into context on demand.
3. **Manual Control**: Use `/skills` in the TUI to browse skills, or `/skills <name>` to view and activate a skill manually.

---

## Configuration Reference

Protonman loads `~/.protonman/config.toml`. When `PROTONMAN_TRUST_PROJECT=1` is set, a project-local `.protonman/config.toml` is merged, with project rules overriding user defaults.

```toml
# Default permission mode: ask | plan | always-approve
[ui]
permission_mode = "ask"

# Permission policy rules
[permission]
default = "ask"

[[permission.rules]]
action = "deny"
tool = "bash"
pattern = "rm -rf *"

[[permission.rules]]
action = "allow"
tool = "read"
pattern = "*.go"

# Workspace boundary & protected file protection
[workspace]
protected_paths = [".env", "secrets/**", "**/*.pem", "**/*.key"]

# OS-level process sandbox profile: off | workspace | read-only | strict
[sandbox]
profile = "off"

# Agent execution boundaries
[agent]
subagents_enabled = true
max_tool_calls = 100
max_live_subagents = 16
max_retained_subagents = 64
subagent_queue_timeout = "30s"
subagent_wait_timeout = "30s"
subagent_max_runtime = "30m"
completed_result_ttl = "24h"

# Optional specialized subagent routes. Provider/model must be set together.
# Omit both to inherit the current Universal model dynamically.
[agent.subagents.strength]
provider = "protonman"
model = "coding-model-id"
reasoning_effort = "medium"

[agent.subagents.agility]
provider = "opencode"
model = "fast-model-id"
reasoning_effort = "low"

[agent.subagents.intelligence]
provider = "anthropic"
model = "reasoning-model-id"
reasoning_effort = "high"

# Shared runtime and network policies
[runtime]
# turn_timeout is disabled by default; set it only when you explicitly want a
# wall-clock ceiling for the entire foreground turn.
round_timeout = "5m"
tool_permission_timeout = "2m"
tool_execution_timeout = "2m"
model_request_timeout = "5m"
model_discovery_timeout = "10s"
webFetchTimeout = "10s"
model_catalog_ttl = "2m"

# Active model preferences
[model]
default = "deepseek-v4-flash-vision-exp"
provider = "protonman"

# Model provider connections
[providers.protonman]
name = "protonman"
type = "openai"
base_url = "https://protonman.dev/api/v1"
api_key = "plk_your_api_key_here"

[providers.opencode]
name = "opencode"
type = "openai"
base_url = "https://opencode.ai/zen/v1"
api_key = ""

[providers.ollama]
name = "ollama"
type = "openai"
base_url = "http://localhost:11434/v1"
api_key = ""

[providers.anthropic]
name = "anthropic"
type = "anthropic"
base_url = "https://api.anthropic.com"
api_key = "your_anthropic_api_key"
```


Execution safety notes:

- `max_tool_calls = 0` disables the cumulative tool-call-count bound; turn and tool timeouts still provide independent safety ceilings.
- `bash` accepts `command`, optional workspace-relative `cwd`, and optional `timeout_seconds` (1-120). A per-call timeout can shorten but never extend the caller/tool-service deadline.
- Bash effect analysis is conservative: proven read-only shell commands may run in plan mode, while mutating or unknown commands remain blocked. Simple redirections/composition and common filesystem/git commands publish proven `affected_paths`; unknown scripts remain fail-closed.
- Bash results preserve compatibility `output` while also exposing bounded `stdout`, `stderr`, per-stream byte counts/truncation flags, exit code, and stable failure codes. Cancellation terminates the command process tree through the sandbox launcher.
- `subagents_enabled = false` disables new delegation by default. The model cannot use `subagent action=spawn`; existing children remain inspectable/waitable/cancelable through `subagent` lifecycle actions until their retained lifecycle records expire.
- `subagents_enabled` is configured through user or trusted-project TOML; changing that policy is intentionally outside the TUI slash-command surface.
- Per-profile `[agent.subagents.strength|agility|intelligence]` tables may route children to a different configured provider/model. `provider` and `model` must either both be present or both be omitted.
- A profile without an explicit provider/model inherits the **current** Universal language model when the child is admitted. Changing `/model` affects future inherited children only; already queued/running children keep their bound model.
- `reasoning_effort` may be configured with or without a model override. Precedence is profile override -> current global `agent.reasoning_effort`/runtime reasoning -> profile default; `auto`/`default` means inherit.
- User and trusted-project subagent tables merge field-wise by canonical profile. Project reasoning-only overrides do not erase a user-level model route, and project model-only overrides do not erase user-level reasoning.
- Configured subagent providers are validated during runtime bootstrap. Missing providers or required credentials fail before delegation starts.
- `subagent action=spawn` starts work asynchronously. Completed child results are delivered automatically to the owning parent turn through event-driven runtime context; normal delegation does not require `wait`, `get`, or `list` polling. The returned `agent_id` remains available for explicit inspection, cancellation, and recovery.
- Child final responses may carry a `<proton-subagent-result>` envelope with `conclusion`, `findings`, and `blockers`. Finding evidence is retained only when it matches successful runtime-observed tool evidence; malformed structured output falls back to plain text, while changed targets and verification remain runtime-derived.
- Spawned work blocks parent completion by default. Set `optional=true` only for speculative work whose result is not required for correctness. An optional result is integrated if it becomes ready in time; otherwise it does not delay the parent and a still-live optional child is canceled when the parent commits its final response. If the parent turn terminates by failure or cancellation, any remaining turn-owned children are canceled because no runtime-context consumer remains.
- `depends_on` accepts already-spawned agent IDs from the same session/parent turn. A dependent child remains queued without consuming a concurrency slot or queue-timeout budget until every dependency completes successfully; a failed/canceled/interrupted dependency fails the downstream child without executing it.
- `subagent_queue_timeout` bounds only admission to concurrency/workspace capacity; queueing never consumes the child runtime budget.
- `subagent_wait_timeout` bounds explicit diagnostic `subagent action=wait` calls. Reaching it returns current lifecycle state and does **not** cancel the child or affect automatic result delivery.
- `subagent_max_runtime` is the hard child-lifetime safety ceiling after execution starts. `subagent action=spawn` `timeout_seconds` may request a shorter ceiling but cannot extend the configured maximum.
- `max_live_subagents` prevents unbounded queued/running work; `max_retained_subagents` caps terminal records even inside the TTL window, while `completed_result_ttl` bounds how long results remain queryable.
- Legacy `subagent_timeout` is accepted as an alias for `subagent_max_runtime` with a deprecation warning.
- `[runtime]` centralizes model, tool, discovery, web-fetch, and catalog-cache time bounds. The loop refuses construction if every global termination bound is disabled.
- Repeating the same deterministic tool call with the same semantic arguments and result twice without an intervening mutation triggers a text-only synthesis round instead of continuing the tool loop; identical retryable failures are capped at three attempts.
- Truncated `read`, `grep`, `find`, and `ls` results include `next_offset` plus a snapshot-bound `continuation`; send both on the next page to detect stale file, query, or directory state. `grep` continuations also carry a validated cursor so deep pages resume near the prior match instead of rescanning earlier files. Plain `offset` remains supported for compatibility. `read` also supports bounded 1-based `start_line`/`end_line` selection with optional `line_numbers` for source inspection without shell `nl`/`sed`.

### Environment Variables

| Variable | Description |
| :--- | :--- |
| `PROTONMAN_HOME` | Override the effective user home beneath which `.protonman/` stores configuration, sessions, checkpoints, skills, and logs |
| `PROTONMAN_TRUST_PROJECT` | Set to `1`, `true`, or `on` to trust and load project-local `.protonman/` configs and skills |
| `PROTONMAN_SESSION_ID` | Explicit session identifier to resume or create |
| `PROTONMAN_SANDBOX` | Override sandbox profile (`off`, `workspace`, `read-only`, `strict`) |
| `PROTONMAN_TELEMETRY` | Set to `stderr` for redacted JSON tool lifecycle and loop-protection telemetry, including suppression, retry-budget, stale-continuation, turn-deadline, and event-driven subagent synthesis byte/batch counters |
| `PROTONMAN_DEBUG_LOG` | Set to a file path or `stderr` for opt-in JSON development diagnostics; disabled by default |
| `PROTONMAN_FORCE_TTY` | Test/development override for terminal detection; normal CLI use should leave it unset |

Legacy `PROTON_TRUST_PROJECT`, `PROTON_SESSION_ID`, `PROTON_SANDBOX`, `PROTON_TELEMETRY`, `PROTON_DEBUG_LOG`, and `PROTON_FORCE_TTY` are accepted only as fallbacks. `PROTONMAN_HOME` is the only supported home override.

---

## Versioning & Releases

Git tags are the source of truth for release versions. `make build` and `make dev`
inject `git describe --tags --always --dirty --match 'v[0-9]*'` into the binary; `VERSION=v1.2.3`
may be supplied explicitly. `protonman --version` reports the version embedded in
the binary.

Pushing a tag such as `v1.2.3` triggers `.github/workflows/release.yml`, which
runs the full test suite, builds Linux amd64 and macOS arm64 archives, generates SHA-256
checksums, and publishes a GitHub Release. Prerelease tags such as `v1.2.3-rc.1`
are published as GitHub prereleases.

See [`docs/releasing.md`](docs/releasing.md) for the release procedure and version
resolution rules.

---

## Development & Testing

```sh
# Run unit and package tests
make test

# Run tests with the Go race detector
make test-race

# Run end-to-end test suite
make test-e2e

# Run benchmarks with memory profiling
make bench

# Format all source files
make fmt

# Run Go vet linter
make vet
```

---

## Contributing

Contributions are welcome through pull requests targeting `main`. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for branch naming, verification, and review expectations. Report suspected vulnerabilities privately as described in [`SECURITY.md`](SECURITY.md).

---

## License

Apache License 2.0. See `LICENSE` for details.
