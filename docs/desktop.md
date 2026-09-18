# Protonman Desktop

Protonman Desktop is a Wails v3 frontend for the existing Protonman runtime. The
desktop application does not embed a second agent loop. It drives the CLI
runtime through the Agent Client Protocol (ACP) JSON-RPC boundary.

## Architecture

```text
protonman-desktop (Wails + React/TypeScript)
            |
            | typed Go bindings and runtime events
            v
      desktop Controller
            |
            | ACP JSON-RPC / stdio
            v
      protonman --acp
```

The CLI runtime remains the source of truth for sessions, tools, models,
permissions, memory, TODO state, and subagents. The desktop controller owns
only client lifecycle, presentation projection, and desktop preferences.

## Development

The frontend uses React 19, TypeScript 7, Vite 8, and the Wails v3 runtime.
Install dependencies and build the embedded React assets:

```sh
bun --cwd cmd/protonman-desktop/frontend install
make desktop-frontend
```

`npm --prefix cmd/protonman-desktop/frontend install` remains supported when Bun
is unavailable. The repository lockfile is `frontend/bun.lock`.

Wails v3 bindings are generated from the registered Go services:

```sh
wails3 generate bindings -ts -f '-tags desktop' \
  -d cmd/protonman-desktop/frontend/bindings ./cmd/protonman-desktop
```

Run the controller tests and build the desktop binary:

```sh
make test-desktop
make desktop
```

The desktop package shell in `cmd/protonman-desktop/main.go` is intentionally
visible to Go tooling and editors. The Wails implementation is in
`main_desktop.go` and is guarded by the `desktop` build tag; `main_stub.go`
provides a clear non-desktop error. Normal CLI builds do not include Wails or
the frontend assets.

Build or run the desktop application with the tag enabled:

```sh
go build -tags desktop ./cmd/protonman-desktop
go run -tags desktop ./cmd/protonman-desktop
```

Wails v3 uses the GTK4/WebKitGTK 6.0 stack by default on supported Linux
distributions. Older distributions must use the documented legacy `gtk3`
build path and matching dependencies.

## Runtime migration status

The Wails shell and framework-free desktop controller are in place. ACP session
supervision, reconnect, permissions, agent management, MCP integrations,
runtime controls, and the complete session UI are being moved behind this
controller incrementally. Until those slices land, the desktop shell exposes
only the migration bridge smoke surface.

The CLI ACP server and existing runtime behavior remain unchanged during this
migration.

## ACP configuration

`PROTONMAN_BINARY` may override the CLI executable used by the desktop runtime.
The release archive continues to bundle the matching CLI under `libexec`.
