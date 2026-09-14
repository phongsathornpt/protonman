# Nerd Fonts Symbols Only

Protonman Desktop uses the Symbols Nerd Font Mono asset from Nerd Fonts for GUI glyphs.

- Upstream: https://github.com/ryanoasis/nerd-fonts
- Version: `v3.5.1`
- Asset: `patched-fonts/NerdFontsSymbolsOnly/SymbolsNerdFontMono-Regular.ttf`
- Git blob SHA: `3b5a184756f6bc44fe77e7c884ec9fda24084505`
- License: MIT, copied in `LICENSE`

The font is fetched during local Desktop preparation and release packaging instead of being stored in this repository. The Desktop resolves it from `share/fonts/` beside the executable. `PROTONMAN_NERD_FONT` may override that path for development or custom packaging.
