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
```

The line-oriented UI is the default for plain terminals and CI. Set
`PROTON_TUI=fullscreen` to use the ANSI full-screen event loop with prompt
editing, scrollback, modal permission prompts, plan mode, and a TODO pane.
Its live-region layout follows Grok Build's minimal pager design: recent output
is bottom-anchored above the TODO panel, activity/status, prompt, and compact
info/shortcut rows. Rendered tool and model text is sanitized before it reaches
the terminal.
Set `PROTON_TELEMETRY=stderr` to emit opt-in JSON lifecycle events for tool
calls and permission decisions. Telemetry contains metadata and argument byte
counts, never raw commands, paths, URLs, arguments, output, or error details.

Inside Proton:

```text
:help
:tools
:call read_file {"path":"README.md"}
:call bash {"command":"pwd"}
:call git_status {}
:call checkpoint_restore {"checkpoint_id":"checkpoint-..."}
:mode always-approve
:mode ask
:quit
```

The default mode is `ask`. A permission prompt accepts `y` for one call, `s`
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

## Verify

```sh
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

The port plan and the next implementation slices live in [TODO.md](TODO.md).
