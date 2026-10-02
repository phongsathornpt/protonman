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

Protonman Desktop is a focused developer workbench. It uses a modern integrated
IDE-style layout with docked edge-to-edge panels and 1px hairline dividers
(`outlineVariant`), replacing isolated floating card islands. The appearance
selector offers macOS Light/Dark palettes and optional Developer Slate Light/Dark
palettes. System Default follows the host appearance and uses the macOS palette.
All themes share semantic color roles, GitHub/VSCode-inspired syntax and diff
highlighting, and Dota-style attribute accents for delegated subagents.

The following patterns are intentionally rejected:

- landing-page heroes, marketing CTAs, and analytics dashboards;
- bulky floating card islands that waste horizontal and vertical screen space;
- glassmorphism, neon gradients, ornamental blobs, and ambient motion;
- emoji used as interface icons;
- hidden focus indicators or pointer-only interactions;
- controls smaller than 44dp touch targets; and
- layouts that introduce horizontal page scrolling.

## Developer Slate Tokens

These tokens define the optional Slate palette. The macOS Light/Dark themes use
the corresponding native palette in `internal/adapter/in/desktop/gioui/theme.go`.
The palettes share semantic roles and retain at least 4.5:1 contrast for text.

| Role | Light | Dark |
| --- | --- | --- |
| Surface | `#F6F8FA` | `#0D1117` |
| Surface dim | `#EAEEF2` | `#080B0F` |
| Surface bright | `#FFFFFF` | `#1C2128` |
| Surface container lowest | `#FFFFFF` | `#080B0F` |
| Surface container low | `#F3F5F8` | `#13171F` |
| Surface container | `#FFFFFF` | `#161B22` |
| Surface container high | `#EBEFF4` | `#21262D` |
| Surface container highest | `#E1E6EB` | `#30363D` |
| On surface | `#1F2328` | `#F0F4F9` |
| On surface variant | `#57606A` | `#9CA3AF` |
| Outline | `#8C959F` | `#484F58` |
| Outline variant (dividers) | `#D0D7DE` | `#21262D` |
| Primary | `#0969DA` | `#58A6FF` |
| On primary | `#FFFFFF` | `#0A1628` |
| Primary container | `#DDF4FF` | `#172B4D` |
| On primary container | `#0969DA` | `#BFDBFE` |
| Secondary container | `#EBF0F4` | `#21262D` |
| On secondary container | `#24292F` | `#E2E8F0` |
| Success container | `#DAFBE1` | `#133820` |
| On success container | `#1A7F37` | `#7EE787` |
| Warning container | `#FFF8C5` | `#3D2E05` |
| On warning container | `#7D4E00` | `#F6E05E` |
| Error container | `#FFEBE9` | `#441B1D` |
| On error container | `#CF222E` | `#FFA198` |
| Tertiary container (thinking) | `#FBEFFF` | `#381E54` |
| On tertiary container | `#6639BA` | `#F3E8FF` |
| Strength container (STR) | `#FFEDD5` | `#431B06` |
| On strength container | `#7C2D12` | `#FDBA74` |
| Agility container (AGI) | `#CCFBF1` | `#0A332C` |
| On agility container | `#115E59` | `#5EEAD4` |
| Intelligence container (INT) | `#EDE9FE` | `#2E1065` |
| On intelligence container | `#4C1D95` | `#DDD6FE` |
| Diff added container | `#DAFBE1` | `#133820` |
| On diff added container | `#1A7F37` | `#7EE787` |
| Diff deleted container | `#FFEBE9` | `#441B1D` |
| On diff deleted container | `#CF222E` | `#FFA198` |

Components use theme roles rather than component-local hex values. Text pairs
must retain at least 4.5:1 contrast in both themes.

## Typography

The client requests `SF Pro, -apple-system, BlinkMacSystemFont, 'Helvetica Neue', Inter, system-ui, sans-serif`
through Gio's system-font shaper and uses developer-oriented typographic hierarchy:

- display small: 30sp semibold for empty-state titles;
- headline small: 20sp semibold for pane and section titles;
- title medium: 16sp semibold for cards, headers, and tabs;
- body large: 16sp regular for primary conversation text;
- body medium: 14sp regular for metadata and supporting content;
- body small: 12sp regular for diff content, code snippets, and logs;
- label large: 14sp semibold for buttons;
- label medium: 12sp regular for supporting controls; and
- label small: 11sp semibold for status pills and diff badges.

Long workspace paths and titles use deterministic truncation rather than
forcing horizontal overflow.

## Workbench Structure & Layout (macOS Split-View Paradigm)

The desktop application is structured as a native macOS 3-column split-view workbench adhering to Apple Human Interface Guidelines:

1. **Unified Top Toolbar (60dp)**: Aligned across panes with the native window titlebar. Contains icon-driven sidebar toggle (`◧`), project/session context, active session title, active agent selector (`▾`), connection status pill, and inspector toggle (`◨`).
2. **Left Sidebar (260dp, 220dp below a 900dp window width, full-height and collapsible)**:
   - Extends the full window height in a quiet `surfaceDim` background.
   - Top bar with the Protonman brand and a compact new-session icon action, thread search on its own row below, and a Projects section label.
   - Collapsible project directory groups with toggle chevrons (`▾`/`▸`) and hover `+` quick action to start new threads directly in that workspace. Expanded empty projects display an inline `"No conversations yet"` + `"+ New"` CTA. Pinned sessions sit under their own collapsible group header above projects.
   - Active thread visual anchoring: prominent 3dp rounded accent indicator bar (`Primary`) on the leading edge of the active session card, filled pill (`primaryContainer` on `onPrimaryContainer`) with an 8dp horizontal gutter, 44dp minimum row height, colored status dots, and relative timestamps. Hover actions (pin, rename, delete) reveal on the active or hovered row, and non-default agents get a compact identity badge. Full arrow key navigation (`UpArrow`/`DownArrow`) traverses between threads and smoothly transitions into search.
3. **Center Conversation Pane (Flexible width, max 840dp text constraint)**:
   - Clean canvas stream: AI assistant messages flow directly onto the canvas (`surface`) without enclosing card borders.
   - User prompts rendered as refined rounded speech bubbles (`shapeLarge` / 12-14dp radius) in elevated `secondaryContainer`.
   - Collapsible thinking blocks with 1-click expand/collapse.
   - Color-coded unified git diff cards with `+adds` / `-dels` badges and syntax-colored lines.
   - Non-diff tool executions rendered as compact step pills.
   - Dota-style attribute cards for delegated subagents (STR, AGI, INT) with 3dp left accent stripes.
   - Floating composer card: elevated container with 16dp corner radius (`shapeExtraLarge`) floating above the bottom, circular Apple-style action button (`↑` send / `■` stop), capsule context chips (Goal, Model), and multiline editor.
4. **Right Inspector Drawer (336dp, collapsible)**:
   - Recessed capsule segmented control tab bar (macOS `NSSegmentedControl` style) in `surfaceContainerLow` with an elevated active tab pill (`surface`).
   - Docked tabs: `[Plan]`, `[Memory]`, and `[Skills]`. General application settings open separately from the sidebar.
   - Grouped cards: 8dp rounded containers (`shapeMedium`) in `surfaceContainer` for Goal, session tasks (TODO checklist), Memory facts, and runtime settings.
5. **Hairline Dividers**: 1px subtle dividers (`outlineVariant`) separate columns cleanly.

## Global Keyboard Shortcuts

| Shortcut | Action |
| --- | --- |
| `Ctrl/Cmd+B` | Toggle left sidebar visibility |
| `Ctrl/Cmd+I` | Toggle right inspector drawer visibility |
| `Ctrl/Cmd+N` | Create a new session in the active workspace |
| `Enter` | Send prompt in composer |
| `Shift+Enter` | Insert new line in composer |

## Shapes

Corner radii adhere to tight developer tooling standards:

- small: 3dp for diff badges, status pills, and code line containers;
- medium: 6dp for buttons, tool cards, and search inputs;
- large: 8dp for panels, composer card, and inspector drawer; and
- extra large: 12dp for modal dialogs and prominent empty-state cards.

## AI Chat and Workbench Features

Protonman Desktop integrates native AI features into the conversation and inspector:

- **Thinking Blocks**: Assistant messages parse `<think>...</think>` into collapsible cards with live streaming indicator and one-click disclosure.
- **Unified Diff Cards**: File edit and patch outputs are detected and styled with file headers, `+add` / `-del` count badges, and green/red line coloring.
- **Subagent Delegations**: Dota-style attribute cards for STRENGTH, AGILITY, and INTELLIGENCE participants with left accent stripes.
- **Active Goal**: Highlighted goal surface in the inspector and composer context chips to orient multi-turn autonomy.
- **Task Plan (TODO)**: Checklist with completion markers (`✓`, `◐`, `○`) and visual linear progress indicator.
- **Durable Memory**: Segmented cards for workspace-local facts and global preferences with confidence ratings.

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
