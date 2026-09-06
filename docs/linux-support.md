# Linux support

Proton supports Linux as a first-class CLI target. The TUI uses terminal detection from `golang.org/x/term`, while process lifecycle and Linux-specific syscalls use Go plus `golang.org/x/sys/unix`.

## Current sandbox backend

Linux filesystem confinement currently uses `bwrap`. Profiles that require confinement fail closed when `bwrap` is unavailable or blocked by the host kernel/security policy. `--sandbox off` does not require `bwrap`.

## Native Go migration

The first native layer is implemented: Proton probes the Linux Landlock ABI and user-namespace configuration directly with Go and `golang.org/x/sys/unix`. Sandbox errors include those capabilities when `bwrap` is unavailable.

The remaining native backend is being introduced in layers: Landlock filesystem rules, network namespaces, and native PTY integration tests. Bubble Tea remains the TUI framework; no TUI rewrite is required.

## Compatibility testing

Linux CI must cover normal Ubuntu, restricted user-namespace environments, real PTY behavior, amd64, arm64, and a no-`bwrap` path as the native backend lands.
