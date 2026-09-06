# Linux support

Proton supports Linux as a first-class CLI target. The TUI uses terminal detection from `golang.org/x/term`, while process lifecycle and Linux-specific syscalls use Go plus `golang.org/x/sys/unix`.

## Current sandbox backend

Linux now uses a native Landlock backend for the `workspace` profile when the kernel exposes Landlock. The policy keeps the host filesystem readable, grants writes only inside the workspace, and preserves the standard writable character devices required by shells. Network-restricted profiles (`read-only` and `strict`) still use `bwrap` until native network namespace isolation lands. `--sandbox off` requires neither backend.

## Native Go migration

Proton probes the Linux Landlock ABI and user-namespace configuration directly with Go and `golang.org/x/sys/unix`. The native child bootstrap applies Landlock before executing the shell, so the parent Proton/TUI process remains unrestricted. Sandbox errors include detected capabilities when a required backend is unavailable.

The remaining native backend is being introduced in layers: network namespaces and native PTY integration tests. Bubble Tea remains the TUI framework; no TUI rewrite is required.

## Compatibility testing

Linux CI must cover normal Ubuntu, restricted user-namespace environments, real PTY behavior, amd64, arm64, and a no-`bwrap` path as the native backend lands.
