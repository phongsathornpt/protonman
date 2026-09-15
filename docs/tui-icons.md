# TUI icon profiles

Protonman's TUI treats terminal icons as a presentation capability, not as application state or project configuration.

## Profiles

The TUI exposes three icon modes through `PROTONMAN_ICONS`:

```text
PROTONMAN_ICONS=auto
PROTONMAN_ICONS=unicode
PROTONMAN_ICONS=ascii
```

`auto` is the default. It does not guess whether the terminal has a patched font. Interactive TUI rendering falls back to the Unicode profile unless the user explicitly selects another profile.

`unicode` uses portable Unicode glyphs and requires no special terminal font installation.

`ascii` avoids private-use and decorative Unicode glyphs for conservative terminal environments.

## Example

## Architecture

Renderers consume semantic fields such as `Icons.Read`, `Icons.Edit`, `Icons.Agent`, and `Icons.ToolSuccess`.

The resolved icon set is injected into TUI presentation state. It is not a mutable package global and is not persisted in `.protonman/config.toml` because a repository must not decide which font a user's local terminal needs.

Raw and copyable transcript output remains text or portable Unicode and should not become part of persisted conversation data, tool results, or execution policy.

## Width contract

For every supported profile:

- prefix icons occupy two terminal cells including their trailing space;
- the compact brand mark occupies one terminal cell;
- layout calculations use display-cell width rather than byte or rune count.
