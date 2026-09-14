# TUI icon profiles

Protonman's TUI treats terminal icons as a presentation capability, not as application state or project configuration.

## Profiles

The TUI exposes four icon modes through `PROTONMAN_ICONS`:

```text
PROTONMAN_ICONS=auto
PROTONMAN_ICONS=nerd
PROTONMAN_ICONS=unicode
PROTONMAN_ICONS=ascii
```

`auto` is the default. It does not guess whether the terminal has a patched font. Interactive TUI rendering falls back to the Unicode profile unless the user explicitly selects another profile.

`nerd` uses the curated Nerd Fonts v3 icon set owned by `internal/adapter/in/tui/view/style`. Protonman supports the **Nerd Font Mono** terminal variant for this mode so icon prefixes retain the existing one-cell glyph plus one trailing-space layout contract.

`unicode` uses the existing portable Unicode glyphs and requires no Nerd Font installation.

`ascii` avoids private-use and decorative Unicode glyphs for conservative terminal environments.

## Example

Configure the terminal emulator to use a Nerd Font Mono font, then launch Protonman with:

```sh
PROTONMAN_ICONS=nerd protonman
```

If the terminal renders missing-glyph boxes, overlapping icons, or unexpected cell widths, switch back without changing project configuration:

```sh
PROTONMAN_ICONS=unicode protonman
```

## Why font detection is explicit

The process running Protonman cannot reliably determine which font the terminal emulator is using. This matters especially over SSH, where Protonman may run remotely while glyph rendering happens in the local terminal.

Terminal-name heuristics such as `TERM`, `TERM_PROGRAM`, tmux, Zed, Kitty, or Ghostty therefore must not be treated as proof of Nerd Font availability.

## Architecture

Raw Nerd Font private-use codepoints belong only to the TUI style presentation leaf. Renderers consume semantic fields such as `Icons.Read`, `Icons.Edit`, `Icons.Agent`, and `Icons.ToolSuccess`.

The resolved icon set is injected into TUI presentation state. It is not a mutable package global and is not persisted in `.protonman/config.toml` because a repository must not decide which font a user's local terminal needs.

Raw and copyable transcript output remains text or portable Unicode. Nerd Font glyphs are presentation-only and should not become part of persisted conversation data, tool results, or execution policy.

## Width contract

For every supported profile:

- prefix icons occupy two terminal cells including their trailing space;
- the compact brand mark occupies one terminal cell;
- layout calculations use display-cell width rather than byte or rune count.

The Nerd profile intentionally contains only the small semantic subset Protonman renders. Do not scatter raw private-use codepoints through individual views.
