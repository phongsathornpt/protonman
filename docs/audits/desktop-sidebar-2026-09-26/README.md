# Desktop sidebar refinement

Visual review of the refined Gio sidebar on 2026-09-26.

## Captures

- `01-native-window.png`: rebuilt desktop app with a populated workspace and 96 conversations.
- `02-light-wide.png`: light component capture at 1180 × 760.
- `03-dark-compact.png`: dark component capture at 760 × 600.

## Review

- Project name and conversation count read as one group; session cards sit below with a clear text indent.
- Recent sessions sort first, and the selected session's activity appears as a quiet subtitle.
- Idle badges are absent. The selected session's `Idle` state remains available in the conversation header, while the sidebar stays focused on titles; attention states still have dedicated labels.
- The compact capture keeps the sidebar usable beside the conversation pane. Long session titles truncate within their row, and project and session rows remain visually separate.
- Older sessions with no persisted activity timestamp have no subtitle. Repeated or generic session titles therefore remain indistinguishable until they gain activity metadata; this reflects missing data rather than unstable ordering.

The component captures are visual evidence for layout and theme behavior. The native screenshot verifies the rebuilt app with real persisted workspace data.
