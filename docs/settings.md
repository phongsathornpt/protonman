# Settings architecture

Protonman treats runtime settings and persisted TOML as separate contracts. The settings refactor must preserve existing `~/.protonman/config.toml` behavior while reducing coupling between runtime types, file schema, defaults, and persistence.

## Ownership

`internal/base/runtimepolicy` owns canonical product runtime defaults. Do not mirror those defaults as aliases in config, TUI, application, or provider packages.

`internal/adapter/out/config` owns layered TOML loading, merge/provenance, file-schema conversion, and user/project persistence. Application-facing mutations remain exposed through `internal/app`.

`DefaultSnapshot()` is the only constructor for a fresh effective config snapshot. Every call must return independent mutable maps and slices.

## Load order

Effective configuration is built in this order:

```text
runtimepolicy defaults
        |
        v
DefaultSnapshot()
        |
        v
user ~/.protonman/config.toml
        |
        v
trusted project .protonman/config.toml
        |
        v
effective Snapshot + provenance + warnings
```

## Persisted TOML boundary

The on-disk schema is a compatibility boundary. `fileDocument` and its nested `file*` structs represent TOML, not runtime state.

Provider and model records therefore use dedicated file-schema types such as `fileProvider` and `fileModel`. Runtime `ProviderConfig` and `ModelConfig` must be populated through explicit conversion when loading and saving.

This keeps runtime refactors from silently changing users' config files.

## Persistence guarantees

User and project settings share the atomic document persistence primitive in `internal/adapter/out/config/store.go`:

```text
read existing document
        |
        v
decode TOML
        |
        v
mutate document
        |
        v
encode to temporary file
        |
        v
set scope-specific permissions
        |
        v
atomic rename
```

The shared primitive does not erase scope-specific security rules. User config remains `0600`; project config remains `0644` and must preserve project-scope and symlink rejection checks.

## Skills configuration and persistence

Active skills can be configured in both user and project configuration:

```toml
[skills]
active = ["my-skill", "another-skill"]
```

When a user activates or deactivates a skill interactively (e.g. via `/skills` or `Ctrl+S`):
- If a project-local `.protonman/` directory exists in the workspace, the active list is persisted to `.protonman/config.toml` (project scope).
- Otherwise, it falls back to `~/.protonman/config.toml` (user scope).

Project configuration takes precedence over user configuration during layered load.

## Current refactor status

Implemented:

- canonical defaults flow through `runtimepolicy` and `DefaultSnapshot()`
- config-specific aliases for runtime defaults are removed
- user/project writes reuse one atomic persistence primitive
- persisted provider/model structs are separated from runtime structs
- existing TOML behavior, file modes, and security checks are preserved

Still intentionally pending:

- split effective settings from load metadata/provenance
- replace the monolithic merge path with decode/patch/apply stages
- move semantic mutations further toward the application layer
- reduce direct inbound-adapter dependencies on the concrete config adapter
- normalize `Config`/`Snapshot` naming only after behavior boundaries are stable

Do not document pending items as implemented behavior.

## Validation

Settings refactors must keep the existing compatibility suite green and should include focused race coverage for config mutation paths. At minimum run:

```text
go test ./...
go test -race ./internal/adapter/out/config -count=1
git diff --check
```
