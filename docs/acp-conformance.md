# ACP Conformance

Protonman targets Agent Client Protocol (ACP) wire protocol version `1` and pins schema artifact release `1.7.0` as the current conformance baseline.

These versions are deliberately tracked separately. ACP can publish multiple schema artifact releases that describe the same wire protocol version, while optional features are negotiated through capabilities.

## Conformance rules

1. A capability must never be advertised unless the corresponding behavior is implemented end to end.
2. Stable ACP behavior takes precedence over `protonman/*` extension methods when both can represent the same semantics.
3. Protonman-specific semantics should use ACP `_meta` first when metadata is sufficient, and a `protonman/*` extension method only when new request/response semantics are required.
4. Unknown ACP extension metadata must be preserved rather than interpreted or discarded.
5. Runtime safety policy remains authoritative even when ACP provides the permission transport.
6. Experimental or preview ACP proposals are not advertised as stable support.

## Implementation status

The machine-readable migration status lives in `internal/adapter/in/acp/conformance.go`. It is intentionally allowed to contain unsupported entries while this branch closes the remaining gaps. An entry may only be marked advertised when it is implemented.

The first implementation milestones are:

- protocol metadata preservation and conformance tests;
- truthful capability negotiation;
- complete stable session lifecycle, beginning with `session/close`;
- standard Session Config Options replacing overlapping Protonman runtime RPCs;
- session usage and session info updates;
- elicitation and authentication;
- request cancellation and reverse client services where supported.

The project should only claim full ACP v1 coverage once all stable agent-side requirements and all advertised optional capabilities are covered by conformance tests and real-client interoperability tests.
