# Proton

Proton is the Go-based coding-agent port of Grok Build. The repository starts
with the execution boundary that the rest of the agent will depend on:

```text
TUI / headless adapters
          |
    application service  -- permission check -->  domain policy
          |
       tool registry  -->  tool handlers (read_file, bash, ...)
```

The first slice is deliberately provider-neutral. It can execute a typed JSON
tool call, evaluates permission rules before any handler runs, and exposes a
small terminal UI for inspecting and exercising the boundary.

## Run

```sh
go run ./cmd/proton
```

Inside Proton:

```text
:help
:tools
:call read_file {"path":"README.md"}
:call bash {"command":"pwd"}
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
```

An omitted rule action defaults to `deny`, matching Grok's fail-closed
configuration behavior. The current workspace's permission mode is restored
from a private file under `~/.proton/sessions/`; set `PROTON_SESSION_ID` to
choose an explicit session key. `PROTON_HOME` can point Proton at an isolated
state/config root for tests or disposable runs.

## Verify

```sh
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

The port plan and the next implementation slices live in [TODO.md](TODO.md).
