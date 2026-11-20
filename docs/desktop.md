# Protonman Desktop

Protonman Desktop is a native Gio frontend for the existing Protonman runtime. It does not embed a second agent loop. The Desktop starts or connects to `protonman --acp` and drives sessions through Agent Client Protocol (ACP) JSON-RPC over stdio.

## Architecture

```text
protonman-desktop-gio (Gio)
  UI + reducer-owned presentation state
            |
            | ACP JSON-RPC / stdio
            v
      protonman --acp
            |
   sessions / models / tools
   memory / goals / TODO
   subagents / permissions / MCP
```

The CLI runtime remains the source of truth for execution. Desktop-owned state is limited to presentation state and client preferences such as the MCP server definitions supplied to ACP sessions.

## ACP agent

Desktop drives exactly one supervised ACP process: `protonman --acp`.
`PROTONMAN_BINARY` overrides the bundled Protonman executable for local
development. There is no multi-agent selection, no third-party ACP support,
and no `PROTONMAN_ACP_AGENTS_JSON` override: Proton Desktop and the Proton CLI
are the only supported runtime.

## Session behavior

Desktop can keep multiple ACP sessions visible while work continues in the background. Session lifecycle is projected as `queued`, `running`, `waiting_permission`, `waiting_user`, `paused`, `completed`, or `failed`.

If the ACP subprocess exits, Desktop reconnects with bounded retry behavior and resumes known sessions. An interrupted in-flight prompt is never replayed automatically because a tool may already have produced side effects before the disconnect.

## Projects and folders

Desktop navigation is project-first. A project is a Desktop-owned collection of
folders; conversations remain owned by the Protonman ACP process
and are shown as recent conversations inside the selected
project. Existing sessions are migrated into a project derived from their
workspace path.

A project can contain multiple folders. The primary folder is sent as ACP
`cwd`; remaining available folders are sent as `additionalDirectories` only
when the connected agent advertises that session capability. Agents without
that capability receive the primary folder only, and Desktop reports this when
creating the session. Folder paths are retained in Desktop preferences when a
checkout is temporarily unavailable, but Desktop refuses to execute a session
until its primary folder exists again.

Desktop calls optional ACP session-list and session-resume methods only when
the agent advertises those capabilities. Agents without session listing use
Desktop's locally retained sessions.

Project identity is separate from Protonman's workspace key. Workspace-keyed
runtime state such as memories and checkpoints remains unchanged by the
project navigation layer.

## Permissions

ACP reverse requests are rendered inline and added to a cross-session permission inbox. Permission decisions are returned using the original JSON-RPC request ID and ACP permission option ID.

Permission modes are Ask (default), Plan (read-only), and Always Approve.

## Tool and subagent activity

Tool calls are normalized into reducer-owned timeline items keyed by `toolCallId` rather than appended as raw log text. Delegated agents are rendered as STRENGTH, AGILITY, and INTELLIGENCE participants from structured Protonman ACP updates.

## Goal, TODO, and memory

Desktop reads durable session context through typed Protonman ACP extensions:

- `protonman/session/context` for active Goal and revisioned TODO state
- `protonman/session/memory` for workspace memory and global preferences
- `protonman/session/memory/forget` to permanently remove wrong or harmful memory

Memory inspection is read-only. Opening the inspector does not record memory usage or trigger extraction.

Forgetting is a separate capability because it changes future model behavior. The
workspace key is resolved server-side from the session, so a client can only
forget memory for the workspace its session is bound to. Workspace scope is the
default; global scope must be requested explicitly because it changes
cross-project behavior.

## Runtime controls

Desktop uses typed control-plane methods instead of injecting slash commands into prompts:

- `protonman/session/runtime`
- `protonman/session/set_model`
- `protonman/session/set_reasoning`
- `protonman/session/set_low_concurrency`

Provider/model and low-concurrency changes rebuild future-turn conversation state through the CLI composition root. Runtime changes are rejected while that session has an active prompt.

## MCP integrations

Desktop can persist client-owned stdio MCP definitions containing:

- name
- command
- argument list
- environment variable names

Environment values are never persisted; they are resolved from the Protonman process environment when ACP payloads are built. The definitions are sent using standard ACP `mcpServers` fields on `session/new`, `session/load`, and `session/resume`. Updating MCP configuration for existing sessions requires an explicit ACP reconnect. Desktop refuses that reconnect while any session is active so a running tool call is not interrupted just to apply configuration.

## Native notifications

Native desktop notifications are not yet implemented by the Gio client. Permission,
completion, and failure events remain visible in the session timeline and permission
inbox; notification parity is tracked separately.

## UI glyphs

Desktop uses portable Unicode and Gio theme rendering for presentation-only glyphs in the native chrome. No external font asset is downloaded or bundled.

## Build

Desktop is guarded by the `desktop` build tag so normal CLI builds do not acquire GUI dependencies.
The package remains importable without the tag for tooling compatibility, but
`gioui.Run` returns an explicit build-tag error until the desktop binary is
built with `-tags desktop`.

```sh
go test -tags desktop ./internal/feature/desktop ./internal/adapter/in/desktop/gioui ./cmd/protonman-desktop-gio
make desktop
make desktop-run
make desktop-gio
make desktop-gio-run
```

Release packaging builds the CLI and Desktop from the same Git tag/version so the bundled ACP runtime and UI stay aligned.

Sessions are also retained in Desktop's local session index. Newly created
sessions appear immediately and are resumed through the Protonman ACP process
after reconnect.

## Scope

The current Desktop milestone covers durable task/session UX, permissions, structured activity, workspace grouping, Goal/TODO/Memory inspection, runtime controls, MCP integrations, reconnect behavior, and portable desktop chrome. Native notifications remain a follow-up item.

Scheduled routines, remote runtime/SSH/sandbox targets, and a richer workspace runtime inspector remain follow-up work.
