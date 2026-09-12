---
name: golang-code-review
description: Automated and systematic Go code quality review. Checks Clean Architecture boundaries, parameter arity, control flow, slice nilness, error wrapping, and line length. Use when asked for "code review", "quality check", "audit go code", or "review pr".
---

# Go Code Quality Review Skill

Expert Go code reviewer specializing in Clean Architecture, idiomatic Go conventions, and automated style enforcement.

## Review Workflow

When performing a Go code review or quality audit:

### Phase 1: Automated Scan
Always run the bundled audit script first to surface syntactic and structural violations across the target path:
```bash
python3 scripts/audit_all.py <target_path>
```
Review findings across:
1. **Function Arity**: functions with >4 parameters (`scripts/check_func_arity.py`).
2. **Control Flow**: unnecessary `else` after `return`/`break`/`continue` and complex boolean conditions (`scripts/check_control_flow.py`).
3. **Slice/Map Nilness**: uninitialized `var s []T` or `var m map[K]V` that may serialize to `null` or panic on write (`scripts/check_slice_nilness.py`).
4. **Line Length**: lines exceeding 120 columns.

### Phase 2: Architecture & Boundary Validation
Check module layering and dependency rules:
- Internal package direction: `cmd` -> `adapter/in` -> `app` -> `engine` -> `core` <- `adapter/out`.
- Run dependency guard tests:
  ```bash
  go test ./test/architecture/...
  ```
- Verify zero imports of outer layers from `internal/core/*` and zero CLI imports in `proton-sdk/*`.
- Consult `references/clean_architecture.md` for architectural boundaries.

### Phase 3: Reliability & Error Handling
Audit error propagation and recovery:
1. **Error wrapping**: verify internal errors are wrapped with `%w` (`fmt.Errorf("context: %w", err)`).
2. **Single handling rule**: verify errors are either logged OR returned, never both.
3. **Ignored errors**: ensure no returned errors are swallowed or discarded with `_`, except in best-effort deferred cleanup.
4. **Panic avoidance**: ensure zero `panic` calls in business logic. `panic` is restricted to compile-time constants at package init.

### Phase 4: Concurrency & Memory Safety
1. **Goroutine bounds**: all goroutines must be bounded by a worker pool, `context.Context`, or `sync.WaitGroup`.
2. **Resource cleanup**: `defer Close()` or cleanup must occur immediately after successful allocation.
3. **Data races**: run race detector on relevant test suites:
  ```bash
  go test -race ./<target_package>/...
  ```
4. **Unsafe & Reflection**: verify `unsafe.Pointer` and `reflect` are absent or strictly isolated to platform syscalls.

### Phase 5: Style & Ergonomics
Consult `references/review_checklist.md` for detailed rules:
- Functions must have $\le 4$ parameters. Group excess parameters into an options struct.
- Context must be first parameter (`ctx context.Context`). Error must be last return value.
- Eliminate unnecessary `else` blocks after terminal keywords.
- Break lines longer than 120 characters at semantic boundaries.

## Parallelizing Audits for Large Codebases
When auditing multiple packages or large repositories, delegate independent packages to `agility` subagents using the `subagent` tool:
- Subagent 1: Audit inbound adapters (`internal/adapter/in/...`).
- Subagent 2: Audit outbound adapters (`internal/adapter/out/...`).
- Subagent 3: Audit core domain & engine (`internal/core/...`, `internal/engine/...`).
- Subagent 4: Audit features & platform (`internal/feature/...`, `internal/platform/...`).
Collect automated reports from each subagent and synthesize findings into an actionable review.
