# Desktop composer optimization

Headless Gio captures of the composer after its compact-layout refinement.

## Captures

- `01-light-compact.png` and `02-dark-compact.png`: 640 × 480.
- `03-light-wide.png` and `04-dark-wide.png`: 1180 × 760.
- `05-native-window.png`: rebuilt desktop app with its live workspace and real session state.

## Review

- At compact width, the composer uses tighter outer and editor padding, a smaller send-button minimum, and hides the normal keyboard hint to return vertical space to the conversation.
- Context labels remain in one row and shorten more aggressively at very narrow widths. The context strip also exposes the full goal, model, and reasoning values through its accessibility description.
- The editor keeps a 56dp compact minimum and can grow to 80dp; wide layouts keep the larger 64–96dp range.
- Busy, loading, and disconnected states retain their explanatory helper text at compact widths.
- Drafts stay isolated by session and are kept in a bounded in-memory cache while the user switches sessions.

These captures verify rendered component layout in both themes. The desktop Gio package and native build are the behavioral and runtime checks.
