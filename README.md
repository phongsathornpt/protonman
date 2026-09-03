# Proton

Proton is the Go-based coding-agent port of Grok Build. The repository starts
with the execution boundary that the rest of the agent will depend on:

```text
TUI / headless adapters       provider model client
          |                           |
          +------ application turn loop
                              |
                 tool-call service -- permission check --> domain policy
                              |
                         tool registry --> tool handlers
```

The first slice is deliberately provider-neutral. It can execute a typed JSON
tool call, evaluates permission rules before any handler runs, and exposes a
small terminal UI for inspecting and exercising the boundary.

## Run

```sh
make tui                      # Start interactive fullscreen TUI (default make target)
go run ./cmd/proton           # Direct go run
go run ./cmd/proton -p '/call read_file {"path":"README.md"}'
go run ./cmd/proton -y -p '/call bash {"command":"pwd"}' --output json
go run ./cmd/proton --acp
go run ./cmd/proton --sandbox strict
```

Run the TUI from an interactive terminal. Without a TTY, Proton refuses to start the fullscreen UI and requires `-p` or `--headless`. Headless runs share the same permission service and fail closed in `ask` mode — pass `-y` or `--permission-mode always-approve` for non-interactive writes.

Set `PROTON_TELEMETRY=stderr` to emit opt-in JSON lifecycle events for tool calls and permission decisions. Telemetry contains metadata and argument byte counts, never raw commands, paths, URLs, arguments, output, or error details.

## Terminal User Interface (TUI)

Proton features a fullscreen terminal user interface built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

### Layout Overview

```text
┌────────────────────────────────────────────────────────────────────────┐
│  Proton ── Go Coding Agent                               [mode: ask]  │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  > User prompt goes here...                                            │
│                                                                        │
│  ● Assistant response streaming with sanitized ANSI formatting...      │
│                                                                        │
│  ⚙ Tool Call: read_file (README.md)                          [SUCCESS] │
│                                                                        │
│  ┌─ [Ctrl+O] TODO Checklist ────────────────────────────────────────┐  │
│  │ [x] 1. Set up project workspace                                  │  │
│  │ [ ] 2. Run test suite                                            │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                        │
├────────────────────────────────────────────────────────────────────────┤
│ ⚠️  Permission Request: bash "rm -rf ./cache"                           │
│    [1] (y) Allow Once                                                  │
│    [2] (s) Allow for Session                                           │
│    [3] (n) Deny                                                        │
├────────────────────────────────────────────────────────────────────────┤
│ > Type a message or '/' for commands...                                │
├────────────────────────────────────────────────────────────────────────┤
│ [Enter] send  [Shift+Tab] mode  [^O] todos  [^T] transcript  [^C] quit │
└────────────────────────────────────────────────────────────────────────┘
```

The live-region layout follows Grok Build's minimal pager design:
- **Header & Mode Row**: Displays current execution mode (`ask`, `plan`, `always-approve`) and active status.
- **Scrollback Viewport**: Streams typed transcript cells (`UserCell`, `AssistantCell`, `ToolCell`, `ErrorCell`, and `SystemCell`). Committed cells are cached for instantaneous redraws.
- **TODO Panel**: Collapsible task checklist toggled via `Ctrl+O`.
- **Permission Modal**: Modal prompt appearing above the composer whenever tool execution requires confirmation.
- **Composer**: Full-featured textarea for composing prompts, running commands, and selecting actions.
- **Footer**: Dynamic status hints and active keybindings.

### Keybindings

| Key | Action |
| :--- | :--- |
| `Enter` | Submit prompt / execute command |
| `Shift+Tab` | Cycle permission mode (`ask` → `plan` → `always-approve`) |
| `Ctrl+O` | Toggle TODO checklist pane |
| `Ctrl+T` | Toggle full raw transcript overlay |
| `Ctrl+L` | Clear screen & reset scrollback |
| `PgUp` / `PgDn` | Scroll viewport history up/down |
| `Ctrl+C` | Cancel active operation or exit |

### In-TUI Slash Commands

Type `/` at the prompt to open the autocomplete command menu, or use colon prefixes (`:help`):

| Command | Description | Example |
| :--- | :--- | :--- |
| `/help`, `:help` | Show available commands and keybindings | `/help` |
| `/tools` | List registered tools and their schemas | `/tools` |
| `/call <tool> <args>` | Execute a tool directly with JSON arguments | `/call read_file {"path":"README.md"}` |
| `/mode <mode>` | Change mode (`ask`, `plan`, `always-approve`) | `/mode always-approve` |
| `/ask` | Switch directly to `ask` mode | `/ask` |
| `/plan` | Switch directly to `plan` mode | `/plan` |
| `/always-approve` | Switch directly to `always-approve` mode | `/always-approve` |
| `!<command>` | Execute shell command directly via `bash` tool | `!git status` |
| `/quit`, `:quit` | Exit Proton cleanly | `/quit` |

### Permission Modes & Modal Controls

Proton enforces security policy boundaries before any tool runs:

- **`ask` (Default)**: Prompts interactively whenever a tool is not explicitly allowed by policy rules.
- **`plan`**: Restricts tool execution to read-only tools and suppresses mutating actions.
- **`always-approve`**: Automatically approves allowed and ask-level tool calls. Explicit policy `deny` rules remain strictly enforced.

When a permission prompt appears in `ask` mode:

| Key / Selection | Action | Scope |
| :--- | :--- | :--- |
| `y` or `1` | Allow Once | Authorizes only this single tool call |
| `s` or `2` | Allow for Session | Authorizes matching calls for the current session without re-prompting |
| `n`, `3`, or `Esc` | Deny | Rejects tool execution (fail-closed) |
| `j` / `k` or `↑` / `↓` | Navigate | Moves selection between options |
| `Enter` | Confirm | Resolves permission with selected option |

## Configuration

Proton loads `~/.proton/config.toml`. A project-local `.proton/config.toml`
is only loaded when `PROTON_TRUST_PROJECT=1` is set.

```toml
[permission]
default = "ask"

[[permission.rules]]
action = "deny"
tool = "bash"
pattern = "rm *"

[[permission.rules]]
action = "allow"
tool = "read"
pattern = "*.md"

[ui]
permission_mode = "ask"

[sandbox]
profile = "off"

[workspace]
protected_paths = [".env", "secrets", "**/*.pem"]
```

An omitted rule action defaults to `deny`, matching Grok's fail-closed
configuration behavior. The current workspace's permission mode is restored
from a private file under `~/.proton/sessions/`; set `PROTON_SESSION_ID` to
choose an explicit session key. `PROTON_HOME` can point Proton at an isolated
state/config root for tests or disposable runs.

File tools are confined to the current workspace, reject traversal and
symlink escapes, and hide configured protected paths from search and listings.
Tool results include stable error codes for headless/model consumers; the TUI
renders those codes when a call fails.
Mutating file tools create a private pre-edit checkpoint and return its ID in
the result; `checkpoint_restore` is permission-gated.
The provider-neutral model boundary is injectable: the application turn loop
streams text, translates model tool calls into `tool.Call` values, executes
them through the permission service, and sends structured results back on the
next model request. It propagates cancellation, supports per-round and
per-tool deadlines, and bounds concurrent read/grep calls without racing
interactive permission prompts. A live provider adapter is still a later port
step.
MCP discovery is also injectable: discovered server tools are registered as
`mcp.<server>.<tool>` with `KindMCP`, while invocations remain behind the same
permission service. Concrete MCP transports are intentionally separate.
The tool-call service also exposes a redacted observer boundary for structured
telemetry; observers receive lifecycle metadata without permission details or
tool results.
Sandbox profiles (`off`, `workspace`, `read-only`, `strict`) confine child
`bash` via `sandbox-exec` on macOS or `bwrap`/`unshare` on Linux, and
`web_fetch` honors the same network policy. A requested confining profile
fails closed when the host cannot enforce it. `--acp` serves line-delimited
JSON-RPC (`initialize`, `session/new`, `session/prompt`) over stdio.

## Verify

```sh
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

The port plan and the next implementation slices live in [TODO.md](TODO.md).
