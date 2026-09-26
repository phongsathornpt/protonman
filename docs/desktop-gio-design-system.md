# Protonman Desktop Gio Design System

This document defines the design and implementation contract for the Gio
desktop client. It is the only desktop frontend; the parity matrix below tracks
the remaining functional and visual validation work.

## First migration slice

The initial Gio client is a real ACP client, not a static mock. It:

- starts `protonman --acp` through `internal/adapter/out/acpclient`;
- performs ACP v1 initialization and reconnects with bounded backoff;
- projects `session/list` into `internal/feature/desktop.State`;
- groups sessions into workspace projects;
- supports keyboard and pointer session selection;
- creates a session from an existing workspace directory; and
- renders live connection, project, session, workspace, and agent metadata.

The Gio client also contains the first conversation workflow: streamed
user/assistant chunks, staged session history, tool and delegated-agent
timeline items, prompting, cancellation, and ACP permission decisions. These
surfaces remain migration-in-progress until running-window and visual parity
checks pass. The Gio client now also projects goal/TODO/memory inspectors and
session runtime controls through the typed ACP extensions. MCP configuration
uses the same client-owned definitions and explicit reconnect semantics. Gio
also supervises configured ACP agent profiles, routes each session through its
owning process, and exposes a progressively disclosed profile editor. Native
notifications remain a follow-up capability.

## Product direction

Protonman Desktop is a focused coding workbench. It uses a Material 3 inspired
structure with neutral graphite surfaces, one cobalt interaction color, and
distinct semantic feedback for connection, success, warning, and failure.
Panels use quiet separation instead of lavender blocks; selected and focused
states carry the visual emphasis.

The following patterns are intentionally rejected:

- landing-page heroes, marketing CTAs, and analytics dashboards;
- glassmorphism, neon gradients, ornamental blobs, and ambient motion;
- emoji used as interface icons;
- hidden focus indicators or pointer-only interactions;
- controls smaller than 44dp; and
- layouts that introduce horizontal page scrolling.

## Material tokens

The palette keeps the cobalt native window chrome in view while removing the
lavender cast from the application surface. Light and dark modes share the
same roles and contrast intent.

| Role | Light | Dark |
| --- | --- | --- |
| Surface | `#F6F7F9` | `#10141A` |
| Surface container | `#FFFFFF` | `#171D25` |
| Surface container high | `#E9EDF2` | `#222A35` |
| On surface | `#17202B` | `#E8EEF6` |
| On surface variant | `#536171` | `#B2BECC` |
| Primary | `#315BE8` | `#9DB7FF` |
| On primary | `#FFFFFF` | `#182F68` |
| Primary container | `#E2E9FF` | `#233B78` |
| On primary container | `#183A9C` | `#DCE6FF` |
| Secondary container | `#E7EBF1` | `#242E3A` |
| On secondary container | `#2D3A4A` | `#D2DCE8` |
| Outline variant | `#C7CFD9` | `#3A4655` |
| Success container | `#E4F5EB` | `#20392A` |
| On success container | `#1B6538` | `#B8F0CA` |
| Warning container | `#FFF1CC` | `#403316` |
| On warning container | `#604600` | `#FFE3A3` |
| Error container | `#FDE9E9` | `#472B30` |
| On error container | `#8C2725` | `#FFDADD` |
| Tertiary container (thinking) | `#F3E8FF` | `#3B1C56` |
| On tertiary container | `#4C1D95` | `#F3E8FF` |
| Strength container (STR) | `#FFEDD5` | `#431B06` |
| On strength container | `#7C2D12` | `#FDBA74` |
| Agility container (AGI) | `#CCFBF1` | `#0A332C` |
| On agility container | `#115E59` | `#5EEAD4` |
| Intelligence container (INT) | `#EDE9FE` | `#2E1065` |
| On intelligence container | `#4C1D95` | `#DDD6FE` |

Components use theme roles rather than component-local hex values. Text pairs
must retain at least 4.5:1 contrast in both themes.

## Typography

The client requests `Roboto, Arial, sans-serif` through Gio's system-font
shaper and uses Material type emphasis:

- display small: 30sp semibold for a single empty-state title;
- headline small: 22sp semibold for window and session titles;
- title medium: 16sp semibold for card and section titles;
- body large: 16sp regular for primary explanatory text;
- body medium: 14sp regular for metadata and supporting content;
- label large: 14sp semibold for buttons; and
- label medium: 12sp regular or semibold for metadata and status.

Long workspace paths and titles use deterministic truncation rather than
forcing horizontal overflow.

## Layout

- Top workspace bar: 64dp minimum height with an 8dp window inset.
- Project/session sidebar: 288dp at wide sizes and 248dp below 900dp.
- Sidebar and main pane: 8dp outer inset with a 12dp separation.
- Interactive controls and session rows: at least 44dp; session rows use 56dp.
- Main content: flexible width with a 680dp maximum empty-state card width.
- Window minimum: 760 by 600dp.
- Default window size: 1180 by 760dp.
- Detail rows collapse from two columns to one column below 480dp available
  width.

## Shapes

Use the shared corner-radius scale from `theme.go` adhering to Material 3:

- small: 8dp for compact inputs and row controls;
- medium: 12dp for buttons, cards, and interactive rows;
- large: 16dp for panels, message bubbles, and workspace surfaces; and
- extra large: 24dp for prominent empty-state cards.

## AI Chat and Workbench Features

Protonman Desktop integrates native AI features into the conversation and inspector:

- **Thinking Blocks**: Assistant messages parse `<think>...</think>` into collapsible Material 3 cards with live status while streaming and one-click disclosure.
- **Active Goal**: Highlighted goal surface in the inspector and composer context chips to orient multi-turn autonomy.
- **Task Plan (TODO)**: Checklist with completion markers (`✓`, `◐`, `○`) and visual linear progress indicator.
- **Durable Memory**: Segmented cards for workspace-local facts and global preferences with confidence ratings.
- **Subagent Delegations**: Dota-style attribute cards for STRENGTH, AGILITY, and INTELLIGENCE participants.

Focus rings use the same radius as their control. Inner surfaces that continue
into adjacent content may stay square so the whole pane reads as one surface.

The project/session sidebar, top workspace surface, selected-session state,
conversation timeline, composer, permission panel, and scrollable inspector
form the current Gio slice. The inspector also contains a progressively
disclosed MCP integration editor with labeled JSON inputs, runtime-only
environment resolution, and an explicit ACP reconnect action. The top workspace
surface owns a keyboard-accessible agent selector; a horizontal, bounded choice
strip appears only while the selector is open. The inspector adds a profile list
and a 44dp-minimum editor for agent ID, display name, command, arguments, and
environment variable names. At narrow widths the inspector is an explicit
conversation/inspector toggle rather than a horizontal page scroll. Later
components must fit this shell without changing its ownership or navigation
model.

## Interaction and accessibility

- Gio `widget.Clickable` provides pointer, Return, and Space activation.
- Tab order follows visual order through the top status surface, sidebar
  actions, session rows, inspector controls, and composer.
- Focused controls receive a visible 2dp primary focus ring.
- Primary actions use a solid cobalt fill; secondary actions use a neutral
  surface; destructive actions use the error role.
- Selected session rows emit `semantic.SelectedOp(true)`.
- Buttons and session rows emit semantic descriptions containing their full
  action context.
- Status is always expressed with text; semantic success, warning, and failure
  colors are supplementary.
- Empty states explain the next action and use the real create-session path.
- Motion must be interruptible, limited to one or two purposeful elements,
  constrained to 150–300ms, and skipped when reduced motion is requested.

## MCP configuration

Gio stores MCP definitions through an application-owned persistence port under
the user configuration preference key `mcp.integrations.v1`. Only environment
variable names are durable; values are resolved from the Protonman process
environment whenever an ACP session payload is built. New, loaded, and resumed
sessions receive standard `mcpServers` fields. Applying changed definitions to
existing sessions requires the explicit reconnect action, which is refused
while any session is active.

## ACP agent profiles

Gio resolves profiles in this order:

1. `PROTONMAN_ACP_AGENTS_JSON`;
2. the application-owned `acp.agents.v1` user preference; then
3. the built-in Protonman `--acp` profile.

Each profile stores an ID, display name, executable, argument vector, and
environment variable names. Processes start through `acpclient.StartCommand`
without a shell. Persisted profiles and the editor store names only; values are
resolved from the Gio process environment at launch and are never persisted.
The process-level `PROTONMAN_ACP_AGENTS_JSON` override may additionally provide
`KEY=value` runtime entries; those values remain in memory only and are never
written to the repository. The override continues to take precedence until it
is removed.

Every profile has an independent supervised client and reconnect loop. New
sessions use the selected project default agent, while existing sessions always
route history, prompts, cancellation, permissions, and ACP events through their
recorded `AgentID`. Disconnecting one process pauses only its sessions and
removes only its permission requests. Goal, TODO, Memory, and runtime controls
remain restricted to Protonman sessions. Profile edits persist immediately but
apply to process launch on the next Desktop restart.

## Parity matrix

| Subsystem | Status | Replacement gate |
| --- | --- | --- |
| Gio window and frame loop | In progress | Build plus running-window validation |
| Material theme and tokens | In progress | Light, dark, and contrast checks |
| Top workspace surface | In progress | Connection and workspace state parity |
| Project/session sidebar | In progress | Selection, creation, reconnect, and empty-state parity |
| Selected-session overview | In progress | Metadata parity and responsive validation |
| Conversation timeline | In progress | Streaming, history, tool, subagent, scroll, and visual parity |
| Composer and cancellation | In progress | Prompt lifecycle, interrupted-turn, and visual parity |
| Permission inbox | In progress | Reverse-request, decision, and visual parity |
| Goal/TODO/Memory inspector | In progress | ACP extension, refresh, and visual parity |
| Runtime controls | In progress | Model, reasoning, low-concurrency, and mutation parity |
| MCP integrations | In progress | Persistence, ACP payload, busy-session refusal, and visual parity |
| Native notifications | Pending | Permission, completion, and failure parity |
| Custom ACP agents/settings | In progress | Persistence, routing, restart, and visual parity |
| Default desktop command | Complete | Gio is the only desktop target; remaining parity checks stay tracked above |

## Build and run

The Gio desktop client is isolated behind the `desktop` build tag and is
the only desktop target.
An untagged import remains available as a compatibility facade, but launching
the UI still requires `-tags desktop`.

```sh
make test-desktop
make test-architecture-desktop
make desktop
make desktop-run
```

`make desktop-gio` and `make desktop-gio-run` remain compatibility aliases.

Environment:

- `PROTONMAN_BINARY`: ACP executable override used by local development.
- `PROTONMAN_ACP_AGENTS_JSON`: complete ACP profile array override; takes
  precedence over user configuration for the lifetime of the process.
- `PROTONMAN_GIO_WORKSPACE`: authoritative existing workspace directory for
  create-session actions; an invalid override fails instead of falling back to
  another directory.
- `PROTONMAN_GIO_THEME`: `light` or `dark`; defaults to `light` until native
  system-theme detection is implemented.

`docs/desktop.md` describes the current Gio desktop client and its operational
contract.
