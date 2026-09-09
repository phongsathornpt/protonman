# Linux support

Protonman supports Linux as a first-class CLI target. The TUI uses terminal detection from `golang.org/x/term`, while process lifecycle and Linux-specific syscalls use Go plus `golang.org/x/sys/unix`.

## Current sandbox backend

Linux now uses a native Landlock backend for the `workspace` profile when the kernel exposes Landlock. The policy keeps the host filesystem readable, grants writes only inside the workspace, and preserves the standard writable character devices required by shells. For `read-only` and `strict`, Protonman combines Landlock with a native user namespace and `CLONE_NEWNET` network namespace when user namespaces are available. `bwrap` remains a fallback for hosts that cannot satisfy the native prerequisites. `--sandbox off` requires neither backend.

## Native Go migration

Protonman probes the Linux Landlock ABI and user-namespace configuration directly with Go and `golang.org/x/sys/unix`. The native child bootstrap applies Landlock before executing the shell, so the parent Protonman/TUI process remains unrestricted. Sandbox errors include detected capabilities when a required backend is unavailable.

The filesystem and network sandbox paths are now native Go/Linux implementations. Linux TUI E2E also uses a real `/dev/ptmx` PTY via `golang.org/x/sys/unix`; Linux TUI E2E no longer relies on `PROTONMAN_FORCE_TTY` (or legacy `PROTON_FORCE_TTY`) to pretend a pipe is a terminal. The remaining portability work is compatibility coverage for restricted kernels and older Linux hosts. Bubble Tea remains the TUI framework; no TUI rewrite is required.

## Compatibility testing

The published Linux release target is currently amd64. Linux CI should exercise normal Ubuntu, restricted user-namespace environments, real PTY behavior, and a no-`bwrap` path for the native backend. Linux arm64 is future compatibility coverage rather than a currently published release target.
