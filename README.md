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
go run ./cmd/proton
go run ./cmd/proton -p '/call read_file {"path":"README.md"}'
go run ./cmd/proton -y -p '/call bash {"command":"pwd"}' --output json
go run ./cmd/proton --acp
go run ./cmd/proton --sandbox strict
```

Proton uses the Bubble Tea full-screen event loop with textarea prompt
editing, viewport scrollback, modal permission prompts, plan mode, and a TODO
pane. The adapter is composed from Bubble Tea, Bubbles (`textarea`,
`viewport`, `spinner`, and `key`), and Lip Gloss layout styles. Run the
TUI from an interactive terminal; without a TTY Proton refuses to start
the fullscreen UI and requires `-p` or `--headless`. Bubble Tea owns raw
input and terminal restore. Headless runs share the same permission
service and fail closed in `ask` mode — pass `-y` or
`--permission-mode always-approve` for non-interactive writes. Session
files under `~/.proton/sessions/` now keep a redacted transcript
(roles, text, tool names) in addition to the permission mode.
The live-region layout follows Grok Build's minimal pager design: a welcome
card, typed transcript blocks, a TODO panel, activity/status, prompt, and a
compact mode/info row. Type `/` for the command menu. Shift+Tab cycles
ask → plan → always-approve. Rendered tool and model text is sanitized before
it reaches the terminal.
Set `PROTON_TELEMETRY=stderr` to emit opt-in JSON lifecycle events for tool
calls and permission decisions. Telemetry contains metadata and argument byte
counts, never raw commands, paths, URLs, arguments, output, or error details.

Inside Proton:

```text
/help
/tools
/call read_file {"path":"README.md"}
/call bash {"command":"pwd"}
/call git_status {}
/call checkpoint_restore {"checkpoint_id":"checkpoint-..."}
/mode always-approve
/always-approve
/mode ask
/quit
```

Colon prefixes (`:help`) remain aliases. `!` on an empty prompt runs `bash`
through the same permission service. The default mode is `ask`. A permission
prompt is an option list (`j`/`k`, `1`–`3`, Enter) with `y` for one call, `s`
for an exact-request grant lasting for the session, and `n` to deny. Explicit
policy denies remain effective even in always-approve mode. Session grants do
not mutate the static policy.

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
