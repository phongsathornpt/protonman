# Architecture boundaries

Proton keeps orchestration thin and moves domain behavior into focused package files. The goal is to avoid god objects while preserving package-level cohesion.

## CLI composition

`cmd/proton/main.go` owns process entry and mode dispatch only. `bootstrap.go` builds the application runtime, `session_runtime.go` owns session resolution, and `headless_mode.go` owns headless execution and persistence.

## Agent coordinator

`internal/agent/coordinator.go` owns coordinator state and construction. Lifecycle, scheduling, retained registry state, events, execution, and runtime controls live in `lifecycle.go`, `scheduler.go`, `registry.go`, `events.go`, `execution.go`, and `control.go` respectively.

## Sandbox

`internal/sandbox/launch.go` validates generic launch requests. Platform dispatch lives in `platform_linux.go`, `platform_darwin.go`, and `platform_other.go`; Linux Landlock/bootstrap and compatibility Bubblewrap code remain isolated from generic orchestration.

## Bubble Tea TUI

`internal/tui/model.go` owns Bubble Tea state and construction. Event routing, command dispatch, model turns, and layout/view behavior are split across `model_update.go`, `model_dispatch.go`, `model_turn.go`, and `model_layout.go`.

## Model catalogs

`internal/tui/model_catalog.go` owns provider-scoped model catalogs, freshness checks, and model identity lookup. The model picker owns fetch generation IDs and cancellation so late responses from an older provider selection cannot mutate the active catalog.

## Model adapters

The OpenAI-compatible adapter separates client configuration, request construction/HTTP dispatch, and streaming decode state into `openai_client.go`, `openai_request.go`, and `openai_stream.go`.

## Refactoring rule

New features should extend the narrowest existing responsibility instead of adding orchestration to entrypoint or coordinator files. Refactors must remain behavior-preserving and should run package tests, race tests for concurrent code, and end-to-end tests before commit.
