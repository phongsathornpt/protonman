# Architecture Regression Contract

Architecture tests protect dependency direction, ownership, and meaningful package topology. They must not freeze incidental implementation details such as ordinary production filenames.

## What the guards enforce

The architecture suite currently protects these categories:

1. **Layer direction**
   - `proton-sdk` does not depend on CLI-owned `internal/*` or `cmd/*` packages.
   - `internal/base/*` remains an innermost dependency-free internal layer.
   - `internal/core/*` does not depend on app, engine, feature, adapter, or composition-root packages.
   - `internal/app/*` does not depend on concrete adapters or the composition root.
   - Package-family rules are boundary-aware: a rule for `internal/app` covers `internal/app/*` but not unrelated names such as `internal/application`.

2. **Ownership**
   - Provider-neutral model contracts such as `Request`, `Response`, `Event`, `Stream`, `ToolCall`, `Usage`, `ModelMetadata`, and `ProviderError` are owned by `proton-sdk`.
   - Outer packages may alias canonical SDK types, but must not redefine competing boundary types.
   - Agent-loop policy does not belong in `proton-sdk`.
   - `internal/*` packages must never depend back on `cmd/*`; composition flows inward from `cmd/protonman`.

3. **Source boundaries**
   - Inbound adapters do not call config persistence APIs directly.
   - TUI code does not call concrete provider-model discovery functions directly.
   - Application ports do not expose concrete agent-coordinator escape hatches.
   - When a boundary has a concrete package/function identity, guards inspect Go syntax and import identity rather than matching raw source text. This avoids false positives from comments, strings, aliases, or similarly named symbols.

4. **Decorator contracts**
   - Model decorators must preserve canonical metadata through `Metadata()`.
   - Adding a new decorator requires explicitly preserving the metadata boundary rather than relying on optional-interface accidents.

5. **Topology where topology is architectural**
   - The `internal/` root contains only the declared architecture groups.
   - The TUI root remains a facade plus `runtime`, `state`, and `view` ownership groups.
   - The application package may add ordinary Go files without updating a filename registry; new application subpackages remain an intentional boundary decision.
   - The model adapter remains one cohesive package unless an intentional architecture change introduces a new subpackage boundary.

6. **Public SDK compatibility**
   - Canonical SDK types and functions compile from an external architecture-test package.
   - Deprecated compatibility surfaces remain compile-checked until an intentional breaking release removes them.

## What the guards must not enforce

Architecture tests should not require an exact list of ordinary `.go` filenames inside a cohesive package. A new file that preserves ownership and dependency direction is not an architecture regression.

Exact file or directory checks are appropriate only when the physical topology is itself part of the contract, for example the `internal/` top-level groups or the TUI facade boundary.

Legacy exact-filename guards should be migrated to semantic ownership/dependency checks when equivalent coverage exists. Do not extend an exact filename whitelist merely to make a valid new implementation file pass.

Raw regular-expression source scans are also a last resort. Prefer package-graph checks for dependencies and AST/import-aware checks for calls or declarations whenever the architecture rule can be expressed semantically.

## Fast validation

Run:

```sh
make test-architecture
```

This runs the architecture suite plus SDK ownership/contract tests. CI executes this after lint and before the complete repository test suite so dependency or ownership regressions fail early.

The architecture harness caches the `go list -json ./...` package graph once per test process. New dependency guards should reuse that graph instead of spawning their own `go list` subprocesses.

The full quality gates remain:

```sh
make lint
make test-architecture
make test
make build
make test-race
```

## Changing the architecture intentionally

When an intentional design change conflicts with a guard:

1. update the architecture documentation first or in the same change;
2. update the semantic contract rather than adding a one-off exception where possible;
3. add a regression test that states the new ownership or dependency rule;
4. update `AGENTS.md` when the rule affects repository-wide contributor behavior;
5. verify `make test-architecture` before running the slower full suite.

A failing architecture test is evidence to review the boundary. It is not automatically evidence that the test should be weakened.
