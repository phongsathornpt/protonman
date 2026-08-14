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

The default mode is `ask`. A permission prompt accepts `y` for one call, `a`
for always approve in the current process, and `n` to deny. Explicit policy
denies remain effective even in always-approve mode.

## Verify

```sh
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

The port plan and the next implementation slices live in [TODO.md](TODO.md).
