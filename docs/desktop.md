# Protonman Desktop

Protonman Desktop is a native Fyne frontend for the existing Protonman runtime. It does not embed a second agent loop. The Desktop starts or connects to `protonman --acp` and drives sessions through Agent Client Protocol (ACP) JSON-RPC over stdio.

## Architecture

```text
protonman-desktop (Fyne)
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

## ACP agent selection

Desktop connects to Protonman by default. To connect to Google Antigravity's
official ACP server, install the platform archive and launch Desktop with:

```sh
PROTONMAN_AGENT=antigravity \
ANTIGRAVITY_ACP_COMMAND=/path/to/agy_acp_server.par \
protonman-desktop
```

The command is executed directly with an argument vector. Optional launcher
arguments can be supplied as a JSON string array:

```sh
ANTIGRAVITY_ACP_ARGS_JSON='["--uid=desktop"]'
```

`PROTONMAN_BINARY` continues to override the bundled Protonman executable when
`PROTONMAN_AGENT` is unset. Antigravity support currently covers the standard
ACP chat, tool, permission, cancellation, MCP, and reconnect path. Protonman
extensions such as Goal/TODO/Memory and runtime controls are unavailable for
non-Protonman agents until capability-aware session support is added.

To run multiple ACP agents at once, provide a JSON profile list or configure them in Agent Settings. Desktop starts
one supervised ACP process per profile; new sessions use the agent selected in
the sidebar ("New with"), while existing sessions display their owning agent, connection
health, and extension capabilities in the conversation header.

```sh
PROTONMAN_ACP_AGENTS_JSON='[
  {"id":"protonman","displayName":"Protonman","command":"protonman","args":["--acp"]},
  {"id":"antigravity","displayName":"Google Antigravity","command":"/path/to/agy_acp_server.par"}
]' protonman-desktop
```

### Automatic ACP CLI Discovery & Agent Manager

Desktop automatically scans the host machine's `PATH` for coding CLI tools that support the Agent Client Protocol:
- **Safe Probing**: Candidates such as `protonman`, `goose`, `zero`, `agy`, `claude`, etc., are probed using bounded execution (`--help`) looking for ACP flags (`acp`, `--acp`).
- **Dedicated Agent Manager Dialog**: Accessible from the sidebar bottom button (`Agents X/Y`), session agent badge, or `⚙ Manage ACP agents…` in the dropdown. Provides a master-detail manager with live process status dots, per-agent hot restart, custom profile creation with executable browser, and default agent selection.
- **Sidebar Quick-Add**: Detected CLIs not yet configured appear directly in the sidebar target agent dropdown (`+ Add <Name> (detected)`). Selecting one immediately configures it and switches the target agent.


## Session behavior

Desktop can keep multiple ACP sessions visible while work continues in the background. Session lifecycle is projected as `queued`, `running`, `waiting_permission`, `waiting_user`, `paused`, `completed`, or `failed`.

If the ACP subprocess exits, Desktop reconnects with bounded retry behavior and resumes known sessions. An interrupted in-flight prompt is never replayed automatically because a tool may already have produced side effects before the disconnect.

## Permissions

ACP reverse requests are rendered inline and added to a cross-session permission inbox. Permission decisions are returned using the original JSON-RPC request ID and ACP permission option ID.

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
- environment list

The definitions are sent using standard ACP `mcpServers` fields on `session/new` and `session/resume`. Updating MCP configuration for existing sessions requires an explicit ACP reconnect. Desktop refuses that reconnect while any session is active so a running tool call is not interrupted just to apply configuration.

## Native notifications

Desktop sends native Fyne notifications when:

- a session requires permission;
- a background task completes; or
- a task fails.

Successful completion notifications are suppressed for the currently active session to avoid duplicating foreground UI feedback. Failure notifications are always emitted and long error text is compacted before delivery.

## UI glyphs

Desktop uses portable Unicode and Fyne theme rendering for presentation-only glyphs in the native chrome. No external font asset is downloaded or bundled.

## Build

Desktop is guarded by the `desktop` build tag so normal CLI builds do not acquire Fyne/CGO requirements.

```sh
go test -tags desktop ./internal/adapter/in/desktop ./cmd/protonman-desktop
make desktop
make desktop-run
```

Linux Fyne builds require the normal OpenGL, X11, and Wayland development packages. Release packaging builds the CLI and Desktop from the same Git tag/version so the bundled ACP runtime and UI stay aligned.

Agents that do not implement `session/list` are supported through Desktop's
local session index. Newly created sessions appear immediately and are resumed
through their owning ACP process after reconnect. Agents without `session/load`
retain the local transcript and can still use `session/resume`.

## Scope

The current Desktop milestone covers durable task/session UX, permissions, structured activity, workspace grouping, Goal/TODO/Memory inspection, runtime controls, MCP integrations, reconnect behavior, native notifications, and portable desktop chrome.

Scheduled routines, remote runtime/SSH/sandbox targets, and a richer workspace runtime inspector remain follow-up work.
