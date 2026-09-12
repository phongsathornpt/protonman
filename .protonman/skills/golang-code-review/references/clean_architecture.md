# Clean Architecture Guidelines for Go

## Layer Hierarchy & Import Direction
1. `internal/base/*`: Leaf utilities. Zero dependencies on any internal package.
2. `internal/core/*`: Pure domain logic, contracts, interfaces. Zero outer layer dependencies (`app`, `engine`, `feature`, `adapter`, `cmd`).
3. `internal/app/*`: Application use-case ports and facades. Inbound adapters consume these ports.
4. `internal/engine/*`: Turn execution, prompt generation, tool calling pipeline.
5. `internal/feature/*`: Domain features (`agent`, `todo`, `skill`, `project`).
6. `internal/platform/*`: Infrastructure primitives (`checkpoint`, `sandbox`, `telemetry`).
7. `internal/adapter/in/*`: Inbound presentation layers (`tui`, `acp`, `headless`). Must not directly call `engine/turn`, `sessionfs`, or coordinator methods.
8. `internal/adapter/out/*`: Driven infrastructure adapters (`config`, `model`, `sessionfs`, `tool/*`).
9. `proton-sdk/*`: Standalone provider-neutral LLM library. Must never import any `cmd/*` or `internal/*` package.
10. `cmd/*`: Composition root. Coordinates bootstrap and wiring.

## Structural Rules
- Always guard package boundaries with architecture dependency tests.
- Keep adapter logic decoupled from core entities.
- Never add persistence logic inside core domain packages.
