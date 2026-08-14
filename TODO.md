# Proton port TODO

This is the working port plan from Grok Build's Rust CLI/TUI into Proton's Go
clean architecture. Each checked item should land in a small, reviewable
commit.

## Port principles

- Keep domain policy and tool-call contracts independent of the TUI, model
  provider, filesystem, and shell implementations.
- Make every external effect pass through the application service so
  permission decisions happen before a handler executes.
- Default to asking for permission. A missing or invalid permission decision
  must fail closed.
- Keep the first implementation boring and observable: explicit interfaces,
  context propagation, typed errors, and table-driven tests.

## Steps

### Step 1 — tool-call and permission foundation (complete in this commit)

- [x] Create the Go module and composition root.
- [x] Define the domain tool contract, call envelope, result, and registry
  boundary.
- [x] Define permission kinds, modes, rules, and deny > ask > allow
  precedence.
- [x] Enforce permission before dispatching a handler.
- [x] Add read-file and shell adapters behind the registry.
- [x] Add an initial terminal UI with tool listing, calls, permission prompts,
  and mode switching.
- [x] Add unit tests for policy matching and application-level enforcement.

### Step 2 — configuration, trust, and session state (complete in this commit)

- [x] Load `~/.proton/config.toml` and project-local `.proton/config.toml`.
- [x] Preserve Grok's rule shape: `action`, `tool`, `pattern`, and
  `pattern_mode`.
- [x] Add session-scoped grants (`allow once`, `allow for session`) without
  mutating the static policy.
- [x] Persist the selected permission mode with a session and restore it
  safely.
- [x] Add project trust gating; untrusted project config is ignored with a
  warning.
- [x] Add protected-path checks; this moves with workspace-root enforcement
  into Step 3.

### Step 3 — coding tools

- [x] Port `write_file`, `search_replace`, `apply_patch`, `grep`, and
  `list_dir` as separate handlers.
- [x] Add workspace-root path resolution, protected paths, and traversal
  protection.
- [x] Add output limits and truncation metadata for read, grep, and listing
  results.
- [x] Add structured failure codes to tool results and permission errors.
- [x] Add bounded `git_status` output as a permission-gated read tool.
- [x] Add atomic edit writes for coding tools and bounded git status. Durable
  checkpoint/restore support remains in the operations slice.

### Step 4 — model and tool loop

- [ ] Add an injectable model client and stream model events.
- [ ] Translate model tool calls into `tool.Call` values and stream progress
  and terminal results back into the turn loop.
- [ ] Add cancellation, timeouts, concurrent read-only calls, and bounded
  execution queues.
- [ ] Add MCP discovery and namespaced tool registration.

### Step 5 — full TUI

- [ ] Replace the line-oriented bootstrap UI with a full-screen event loop.
- [ ] Add scrollback, prompt editing, tool progress, modal permission views,
  plan mode, and a TODO pane.
- [ ] Keep the TUI as an adapter: it must not own policy or execute tools
  directly.

### Step 6 — safety and operations

- [ ] Add OS-level sandbox profiles and network restrictions.
- [ ] Add redacted structured telemetry for tool and permission events.
- [ ] Add headless/ACP entry points and session persistence.
- [ ] Run race, fuzz, integration, and end-to-end tests in CI.

## Grok Build mapping

| Grok Build area | Proton destination |
| --- | --- |
| `xai-tool-runtime` | `internal/domain/tool` + `internal/application/toolcall` |
| workspace permission policy | `internal/domain/permission` |
| `xai-grok-tools` registry | `internal/adapters/tools` |
| `xai-grok-pager` | `internal/adapters/tui` |
| shell/workspace composition | `cmd/proton` |

The first commit deliberately stops before model transport, MCP, and OS
sandboxing. Those features should build on the tested permission and dispatch
boundary instead of creating a second execution path.
