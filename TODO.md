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
- [x] Add atomic edit writes for coding tools and durable checkpoint/restore
  support.

### Step 4 — model and tool loop

- [x] Add an injectable model client and stream model events.
- [x] Translate model tool calls into `tool.Call` values and stream progress
  and terminal results back into the turn loop.
- [x] Add cancellation, timeouts, concurrent read-only calls, and bounded
  execution queues. Interactive permission modes remain serialized; bounded
  parallel reads are enabled only for `always-approve` mode.
- [x] Add injectable MCP discovery and namespaced `mcp.<server>.<tool>` tool
  registration. Concrete MCP transports remain adapter-specific.

### Step 5 — full TUI

- [x] Make Bubble Tea the only terminal adapter and run the fullscreen UI on
  every launch.
- [x] Build the fullscreen adapter on Bubble Tea with Bubbles components and
  Lip Gloss styles for terminal lifecycle, input, viewport, and layout.
- [x] Add scrollback, prompt editing, tool progress, modal permission views,
  plan mode, and a TODO pane.
- [x] Port the minimal live-region hierarchy: bottom-anchored output, bounded
  TODOs, activity/status, prompt info, and shortcut rows.
- [x] Keep the TUI as an adapter: it must not own policy or execute tools
  directly.
- [x] Port Grok-feel chrome: GrokNight theme, welcome card, `/` slash menu
  (colon alias), Shift+Tab mode cycle, option-list permissions, typed
  transcript blocks, `!` bash prefix, and streamed turn events.

### Step 6 — safety and operations

- [x] Add OS-level sandbox profiles and network restrictions.
- [x] Add redacted structured telemetry for tool and permission events.
- [x] Add a headless `-p` / `--headless` entry point that uses the same
  tool-call service. Ask mode stays fail-closed (no interactive prompt).
- [x] Persist the conversation transcript with the session (content and tool
  names only; never tool arguments).
- [x] Run `go vet`, `go test`, and `go test -race` in CI.
- [x] Add a concrete ACP transport and fuzz / end-to-end suites.

### Security hardening follow-up

Issues found during the post-port security/code-smell review. Keep these as
separate, regression-tested changes rather than broad refactors.

- [x] Make macOS writable sandbox profiles deny host writes outside the
  workspace before granting the workspace subtree.
- [x] Make every Linux confining profile fail closed when `bwrap` is
  unavailable instead of falling back to a bare shell.
- [x] Expose the host runtime read-only inside Bubblewrap so `sh`, loaders,
  Git, and normal build tools remain usable while only the workspace is
  writable.
- [x] Normalize domain permission patterns so `PatternModeDomain` is truly
  case-insensitive for both allow and deny rules.
- [x] Make leading `**/` protected-path globs match root-level files as well as
  nested files; cover `server.pem`, `certs/server.pem`, and deeper paths.
- [ ] Implement real ACP `session/cancel`: keep per-session cancel functions
  and decouple input reading from a running prompt so cancellation can be
  processed while work is active.
- [ ] Harden filesystem mutations against symlink TOCTOU between path checks
  and `Open`/`MkdirAll`/`CreateTemp`/`Rename`; prefer descriptor-relative or
  no-follow operations where supported.
- [ ] Add OS integration tests that execute the sandbox boundary, including
  denied writes outside the workspace, allowed workspace writes, read-only
  denial, runtime command availability, and network blocking.

## Grok Build mapping

| Grok Build area | Proton destination |
| --- | --- |
| `xai-tool-runtime` | `internal/domain/tool` + `internal/application/toolcall` |
| workspace permission policy | `internal/domain/permission` |
| `xai-grok-tools` registry | `internal/adapters/tools` |
| `xai-grok-pager` | `internal/adapters/tui` |
| shell/workspace composition | `cmd/proton` |

The provider-neutral turn loop still stops before a live model transport.
Sandbox, headless, and ACP all go through the same permission-aware tool
service instead of creating a second execution path.
