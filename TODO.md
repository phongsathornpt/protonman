# Proton CLI TODO

## P1 — Tool contract correctness
- [x] Precompile/cache `InputSchema` + `OutputSchema` per registered tool
- [x] Preserve `StructuredOutput` across result budgeting
- [x] Migrate all agent lifecycle tools to `StructuredOutput` + `OutputSchema`
- [x] Add model-specific schema lowering for Gemini/OpenAI compatibility
- [x] Add MCP contract-violation diagnostics

## P2 — Contract hardening
- [x] Set `additionalProperties: false` for built-in tool schemas
- [x] Normalize optional-zero semantics
- [x] Add generic contract tests for every registered built-in tool

## Existing TUI branding work
- [x] [tui-logo-1] Add brand mark: `glyphBrand` in `theme.go` + `brand.go` with `brandLockup()` renderer
- [x] [tui-logo-2] Wire `brandLockup` into `welcomeCard()`, compact cwd/model lines to preserve footprint
- [x] [tui-logo-3] Narrow-terminal fallback: drop mark below `minBrandWidth` (24 cols), keep wordmark — implemented in `brandLockup()`
- [~] [tui-logo-4] Add `brand_test.go`: width assertion, ANSI SGR check, fallback path
- [ ] [tui-logo-5] Consistency sweep: `crash_view.go` footer gets brand mark
- [ ] [tui-logo-6] Verify: `go test ./internal/tui/...`, `go build`, `make lint`
