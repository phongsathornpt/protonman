# Proton

Proton is an autonomous, high-performance Go-based AI coding agent designed with clean architecture, strict security boundaries, fail-closed permission policies, and extensible agent skills.

Proton operates across multiple execution environments:
- **Interactive TUI**: A terminal interface built with Bubble Tea, featuring live streaming, markdown formatting, collapsible task panes, and modal approval controls.
- **Headless CLI**: A scriptable runner supporting one-shot prompts, piped input via stdin, and structured text or JSON output.
- **ACP Server**: An Agent Client Protocol server serving line-delimited JSON-RPC over stdio for IDE and editor integrations.

---

## Architecture Overview

Proton isolates external effects behind strict application boundaries. External commands and file modifications must pass policy checks and pre-edit checkpointing before execution.

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

### Prerequisites
- Go 1.27+ installed
- Git
- Linux sandboxing uses native Landlock and network namespaces when supported; `bwrap` is an optional fallback

### Running Proton

```sh
# Start the interactive fullscreen TUI (default)
make tui
# Or run directly with Go
go run ./cmd/proton

# Build the standalone binary
make build
./bin/proton
```

### Headless & Scripting

```sh
# Run a single prompt and exit
proton -p "Explain the project architecture"

# Run non-interactively with auto-approval (always-approve mode)
proton -y -p "Run the test suite and fix any failing tests"

# Read prompt from stdin and output JSON
cat prompt.txt | proton --headless --output json

# Serve Agent Client Protocol (ACP) over stdio
proton --acp

# Run within a strict OS sandbox profile
proton --sandbox strict -p "Analyze dependencies"
```

---

## Terminal User Interface (TUI)

The fullscreen TUI is built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

### Live Interface Layout

```text
┌────────────────────────────────────────────────────────────────────────┐
│  Proton ── Go Coding Agent                               [mode: ask]  │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  > Analyze repository structure and test coverage                      │
│                                                                        │
│  ● Assistant response streaming with sanitized ANSI formatting...      │
│                                                                        │
│  ⚙ Tool Call: read_file (internal/config/config.go)          [SUCCESS] │
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
│ [Enter] send  [Shift+Tab] mode  [^P] models  [^S] skills  [^C] quit   │
└────────────────────────────────────────────────────────────────────────┘
```

### Keybindings

| Key | Action |
| :--- | :--- |
| `Enter` | Submit current prompt / execute command |
| `Shift+Tab` | Cycle permission mode (`ask` → `plan` → `always-approve`) |
| `Ctrl+P` | Toggle Model & Provider selector pane |
| `Ctrl+S` | Toggle Agent Skills browser pane |
| `Ctrl+O` | Toggle Tasks / TODO checklist pane |
| `Ctrl+T` | Open transcript overlay (toggle raw view with `r`) |
| `Ctrl+L` | Clear the visible viewport transcript |
| `PgUp` / `PgDn` | Scroll transcript viewport up/down |
| `Esc` | Park active permission modal or dismiss overlays |
| `Ctrl+C` | Cancel active operation or exit |

### In-TUI Slash Commands

Type `/` at the prompt to trigger autocomplete, or prefix with a colon (`:help`):

| Command | Description | Example |
| :--- | :--- | :--- |
| `/help`, `:help` | Display available commands and keybindings | `/help` |
| `/model [name]` | Open model selector or switch active model (`/models` is an alias) | `/model glm-5.3-flash` |
| `/provider [cmd]` | Manage and configure AI model providers | `/provider list`, `/provider opencode` |
| `/tools` | List registered tools and parameter schemas | `/tools` |
| `/skills` | List discovered Agent Skills | `/skills` |
| `/skill <name>` | Inspect or activate a specific Agent Skill | `/skill pdf-processing` |
| `/call <tool> <json>` | Directly execute a tool with JSON arguments | `/call read_file {"path":"README.md"}` |
| `/mode <mode>` | Switch permission mode (`ask`, `plan`, `always-approve`) | `/mode plan` |
| `/ask` | Switch directly to `ask` mode | `/ask` |
| `/plan` | Switch directly to read-only `plan` mode | `/plan` |
| `/always-approve` | Switch directly to `always-approve` mode | `/always-approve` |
| `/new` | Clear conversation history and start a fresh session | `/new` |
| `!<command>` | Execute a shell command directly through the `bash` tool | `!git status` |
| `/quit`, `:quit` | Exit Proton cleanly | `/quit` |

The model picker keeps catalogs scoped per configured provider. Fresh catalogs are cached briefly, stale or missing catalogs are refreshed, obsolete requests are canceled, and late responses from an older provider selection are ignored. Press `/` inside the model picker to filter by model ID, name, vendor, or feature; `r` forces a refresh. Direct `/model <id>` selection still permits custom/unlisted model IDs and marks them as unverified instead of rejecting them.

---

## Security & Policy Engine

Proton enforces security policy boundaries before any tool executes.

### Permission Modes

- **`ask` (Default)**: Prompts interactively whenever a tool action is not explicitly pre-approved by configuration rules.
- **`plan`**: Read-only mode. Mutating file tools (`write_file`, `search_replace`, `apply_patch`) and non-whitelisted shell commands are blocked.
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
- **Automatic Checkpoints**: Mutating file operations create pre-edit snapshots stored under `~/.proton/checkpoints/`. File state can be restored via `checkpoint_restore`.

### OS Sandbox Profiles

Proton can confine sub-processes via OS-level sandboxing:
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
proton --sandbox strict -p "Analyze local files"
```

### Tool Deadlines

Permission resolution and approved tool execution have separate two-minute
deadlines by default. A shorter parent turn or tool context still wins. Bash
commands receive context cancellation, preserve partial output, and terminate
their process tree where the platform supports it. Set `PROTON_DEBUG_LOG` to
`stderr` or a file path to inspect timeout cause, error type, and process
termination diagnostics without logging command contents.

---

## Model Providers & Catalog

Proton connects to OpenAI-compatible endpoints with support for custom gateways and local inference:

### Built-in Provider Presets

| Provider | Base URL | Auth Required | Description |
| :--- | :--- | :--- | :--- |
| **OpenCode** | `https://opencode.ai/zen/v1` | No (Free) | Free-tier models with zero API key required |
| **Protonman** | `https://protonman.dev/api/v1` | Yes (`plk_...`) | High-speed AI model gateway |
| **Ollama** | `http://localhost:11434/v1` | No | Local LLM inference |
| **OpenAI** | `https://api.openai.com/v1` | Yes (`sk-...`) | Official OpenAI API |

### Available Models (Protonman Gateway)

- `deepseek-v4-flash-vision-exp` (Default, 1M context, tool-calling & vision)
- `glm-5.3-flash` (1M context, tool-calling & vision)
- `Qwen3.8-Flash` (1M context, text & vision)
- `muse-spark-1.3-contributor` (1M context, tool-calling)
- `MiniMax-M3` (1M context)

Configure providers directly inside the TUI with `/provider` or via `~/.proton/config.toml`.

---

## Agent Capabilities & Tools

Proton registers a suite of workspace-safe tools:

| Tool | Category | Description |
| :--- | :--- | :--- |
| `read_file` | File System | Read UTF-8 workspace files with boundary-safe byte `offset`/`limit` pagination plus snapshot-bound `next_offset`/`continuation` |
| `write_file` | File System | Write file contents with automatic pre-edit checkpointing |
| `search_replace` | File System | Exact block replacement in files with pre-edit checkpointing |
| `apply_patch` | File System | Apply unified diff patches with pre-edit checkpointing |
| `grep` | Search | Regex search with include globs plus snapshot-bound cursor pagination that resumes from the prior match location |
| `list_dir` | Search | List visible directory entries with protected-path filtering and snapshot-bound pagination |
| `git_status` | Version Control | Inspect Git working tree state and uncommitted changes |
| `bash` | Execution | Run bounded shell commands with workspace-relative `cwd`, optional `timeout_seconds`, effect analysis, and structured stdout/stderr |
| `web_fetch` | Network | Retrieve remote web pages conforming to sandbox network policy |
| `activate_skill` | Skills | Dynamically load an Agent Skill's full context into the session |
| `get_todo` | Tasks | Read the current parent-owned task snapshot and revision |
| `update_todo` | Tasks | Atomically replace parent-owned task state using `expected_revision` from `get_todo` to reject stale updates |
| `delegate_task` | Multi-Agent | Spawn a persistent background subagent and return its `agent_id` immediately |
| `wait_agent` | Multi-Agent | Wait briefly for a subagent; wait timeout leaves the child running |
| `get_agent` | Multi-Agent | Inspect one retained subagent and terminal result |
| `list_agents` | Multi-Agent | List queued, running, and retained terminal subagents |
| `cancel_agent` | Multi-Agent | Explicitly cancel a queued or running subagent |
| `checkpoint_restore` | Recovery | Rollback a file to a recorded pre-edit checkpoint ID |

---

## Agent Skills

Proton implements the open [Agent Skills Specification](https://agentskills.io). Skills are self-contained directory packages containing a `SKILL.md` (YAML frontmatter + Markdown instructions) and optional helper scripts and references.

### Discovery Locations
- **User-level**: `~/.proton/skills/` and `~/.agents/skills/`
- **Project-level**: `<workspace>/.proton/skills/` and `<workspace>/.agents/skills/`

> [!NOTE]
> Project-local skills and configuration are only loaded when `PROTON_TRUST_PROJECT=1` is enabled. Untrusted project skills are safely skipped with a diagnostic warning.

### Progressive Disclosure
1. **Catalog (Tier 1)**: Available skills are summarized as `<available_skills>` in the system prompt (~50-100 tokens per skill).
2. **Activation (Tier 2)**: When a task matches a skill, the model invokes `activate_skill`, loading full instructions, scripts, and asset references into context on demand.
3. **Manual Control**: Use `/skills` in the TUI to browse skills, or `/skill <name>` to view and activate a skill manually.

---

## Configuration Reference

Proton loads `~/.proton/config.toml`. When `PROTON_TRUST_PROJECT=1` is set, a project-local `.proton/config.toml` is merged, with project rules overriding user defaults.

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
tool = "read_file"
pattern = "*.go"

# Workspace boundary & protected file protection
[workspace]
protected_paths = [".env", "secrets/**", "**/*.pem", "**/*.key"]

# OS-level process sandbox profile: off | workspace | read-only | strict
[sandbox]
profile = "off"

# Agent execution boundaries
[agent]
max_rounds = 20
max_tool_calls = 100
max_live_subagents = 16
max_retained_subagents = 64
subagent_queue_timeout = "30s"
subagent_wait_timeout = "30s"
subagent_max_runtime = "30m"
completed_result_ttl = "10m"

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
```


Execution safety notes:

- `max_rounds = 0` disables only the round-count bound; `max_tool_calls = 0` disables only the cumulative tool-call-count bound.
- `bash` accepts `command`, optional workspace-relative `cwd`, and optional `timeout_seconds` (1-120). A per-call timeout can shorten but never extend the caller/tool-service deadline.
- Bash effect analysis is conservative: proven read-only shell commands may run in plan mode, while mutating or unknown commands remain blocked. Simple redirections/composition and common filesystem/git commands publish proven `affected_paths`; unknown scripts remain fail-closed.
- Bash results preserve compatibility `output` while also exposing bounded `stdout`, `stderr`, per-stream byte counts/truncation flags, exit code, and stable failure codes. Cancellation terminates the command process tree through the sandbox launcher.
- `delegate_task` starts work asynchronously. The returned `agent_id` can be used with `wait_agent`, `get_agent`, or `cancel_agent` in the same Proton session.
- `subagent_queue_timeout` bounds only admission to concurrency/workspace capacity; queueing never consumes the child runtime budget.
- `subagent_wait_timeout` bounds one `wait_agent` call. Reaching it returns the current `queued`/`running` state and does **not** cancel the child.
- `subagent_max_runtime` is the hard child-lifetime safety ceiling after execution starts. `delegate_task.timeout_seconds` may request a shorter ceiling but cannot extend the configured maximum.
- `max_live_subagents` prevents unbounded queued/running work; `max_retained_subagents` caps terminal records even inside the TTL window, while `completed_result_ttl` bounds how long results remain queryable.
- Legacy `subagent_timeout` is accepted as an alias for `subagent_max_runtime` with a deprecation warning.
- A complete model/tool turn still has a default 10-minute deadline, and the loop refuses construction if every global termination bound is disabled.
- Repeating the same deterministic tool call with the same semantic arguments and result twice without an intervening mutation triggers a text-only synthesis round instead of continuing the tool loop; identical retryable failures are capped at three attempts.
- Truncated `read_file`, `grep`, and `list_dir` results include `next_offset` plus a snapshot-bound `continuation`; send both on the next page to detect stale file, query, or directory state. `grep` continuations also carry a validated cursor so deep pages resume near the prior match instead of rescanning earlier files. Plain `offset` remains supported for compatibility.

### Environment Variables

| Variable | Description |
| :--- | :--- |
| `PROTON_HOME` | Custom root directory for configuration, sessions, and checkpoints |
| `PROTON_TRUST_PROJECT` | Set to `1`, `true`, or `on` to trust and load project-local `.proton/` configs and skills |
| `PROTON_SESSION_ID` | Explicit session identifier to resume or create |
| `PROTON_SANDBOX` | Override sandbox profile (`off`, `workspace`, `read-only`, `strict`) |
| `PROTON_TELEMETRY` | Set to `stderr` for redacted JSON tool lifecycle and loop-protection telemetry, including suppression, retry-budget, stale-continuation, and turn-deadline counters |
| `PROTON_DEBUG_LOG` | Set to a file path or `stderr` for opt-in JSON development diagnostics; disabled by default |

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

## License

MIT License. See `LICENSE` for details.
