# Go Code Quality & Style Review Checklist

## 1. Function Design
- Max 4 parameters: Functions with >4 parameters should group related parameters into an options struct or use functional options.
- Context first: `context.Context` must always be the first parameter when present.
- Error last: `error` must be the last return value.
- Parameter order: `ctx`, inputs, outputs/destinations.

## 2. Control Flow & Cyclomatic Complexity
- Eliminate unnecessary `else`: Drop `else` after blocks ending in `return`, `break`, or `continue`.
- Extract complex conditionals: If a condition has 3+ boolean operands, extract into well-named local boolean variables.
- Early return: Keep the happy path at minimal indentation. Check guard conditions and errors first.

## 3. Variable & Collection Declarations
- Explicit slice/map initialization: Use `s := []T{}` or `make([]T, 0, cap)` instead of `var s []T` when serializing to JSON to avoid `null` outputs.
- Named fields in composite literals: Always specify field names when constructing structs (`Foo{Bar: 123}`).
- Short declaration vs var: Use `var` for zero-value intent, `:=` for non-zero initialization.

## 4. Error Handling & Reliability
- Wrap with context: Use `fmt.Errorf("doing thing: %w", err)` for internal error propagation.
- Single handling rule: Log the error OR return it, never both.
- Check errors: Never discard non-cleanup errors with `_`.
- Panic avoidance: Never panic in library or runtime business logic. Reserve for unrecoverable programmer errors during package init.

## 5. Concurrency & Resource Lifecycle
- Goroutine lifecycle: Goroutines must be bounded by a context, worker pool, or `sync.WaitGroup`.
- Immediate cleanup: `defer Close()` or `defer cleanup()` immediately after successful resource acquisition.
- Mutex lock duration: Keep critical sections minimal. Do not hold locks across I/O or network requests.

## 6. Formatting & Conventions
- Line length: Break lines beyond ~120 characters at semantic boundaries.
- Multiline calls: Calls with 4+ arguments should wrap one argument per line.
- Imports: Group standard library, third-party, and internal packages cleanly. No dot imports. Blank imports restricted to main/init.
