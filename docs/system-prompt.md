# System Prompt Architecture

Protonman builds one provider-neutral, capability-driven system prompt in `internal/engine/prompt` for every model request. The prompt is intentionally deterministic and ordered for prefix-cache reuse. Provider adapters may encode the resulting message differently on the wire, but they must not silently change its semantics.

## Prompt ABI

The current managed prompt format is **Prompt ABI v12**:

```text
<proton-system-prompt version="12">
...
</proton-system-prompt>
```

The Prompt ABI version identifies Protonman's managed prompt format and ordering contract. It is not the Proton SDK API version and does not imply an SDK major-version change.

Bump the Prompt ABI when a change intentionally alters the managed prompt's model-facing contract, section topology, or serialization in a way that should invalidate assumptions about an older prompt shape.

## Cache-aware section topology

Sections are registered as deterministic `prompt.Section` values, sorted first by numeric order and then by section name. Sparse orders are used so future sections can be inserted without moving volatile material toward the beginning of the prompt.

The current topology is intentionally ordered from the most reusable material toward more request-specific material:

```text
Identity
Execution Contract
Tool Use
Task Coordination          (when task tools are visible)
Delegation Protocol        (when agent tools are visible)
External MCP Tools         (when MCP tools are visible)
Editing And Verification   (when workspace mutation is possible)
Model Guidance             (when the model profile supplies hints)
Grounding Contract         (when grounding is required)

Project Instructions
Additional Instructions
Skills
Role                       (subagents / scoped roles)
Active Goal
Workspace                  (most volatile built-in section)
```

The exact numeric order is an internal implementation detail. The behavioral invariant is that stable, reusable instructions precede progressively more dynamic context unless instruction precedence requires otherwise.

## Prefix-cache invariants

Changes should preserve these invariants:

1. Equivalent semantic inputs render byte-identical prompt output.
2. Section order never depends on map iteration, plugin registration order, or other nondeterministic runtime state.
3. Volatile values such as workspace paths remain after reusable system, capability, and project instructions when semantics permit.
4. Changing one dynamic section should not change bytes before that section's intended cache boundary.
5. Tool guidance reflects the effective tool surface. The prompt must not advertise a capability the model cannot call.
6. Runtime enforcement remains authoritative. Prompt wording never substitutes for permission, sandbox, safety, grounding, or mutation enforcement.

These rules improve prefix reuse for providers and runtimes that implement KV/prefix caching. They are still useful when a provider does not expose cache statistics because deterministic prompt construction reduces accidental request drift.

## Dynamic inputs

The current renderer accepts dynamic fields through `prompt.Spec`, including model hints, project instructions, skills, role, active goal, workspace, grounding requirements, and the effective tool surface.

The active goal is durable session state, not merely a compaction hint. When present, it is
the persistent objective for the session: the model should continue making concrete progress
until the goal is completed, blocked by unavailable capabilities or permissions, or explicitly
changed or cleared. Implementation goals require repository inspection, mutation, and
verification rather than a plan-only response. The TUI `/goal <detail>` command starts the
execution turn; prompt wording does not itself schedule a turn.

Not all of these fields have the same stability. Prefer keeping highly reusable contracts early and request/session-specific material late. Do not interpolate timestamps, generated IDs, or other per-request noise into an early prompt section.

Future work may move appropriate session transitions into append-only history/context instead of rewriting the managed system prompt. Such a change must preserve runtime enforcement and be covered by behavioral regression tests.

## Tools and schemas

System-prompt determinism alone does not guarantee provider prefix-cache reuse. Provider-facing tool definitions are part of the effective model input on many serving stacks. Tool definitions therefore require deterministic publication as well.

When changing tool publication:

- keep canonical tool names and schemas stable when capability semantics are unchanged;
- avoid arbitrary order changes;
- do not expose unauthorized tools merely to improve cache reuse;
- prefer stable capability profiles for subagents over ad hoc tool sets when that does not weaken isolation or correctness.

## Subagents

Subagents reuse the common Protonman execution/tool contracts and append specialization through the later `Role` section. This intentionally keeps the shared parent/subagent prefix as large as semantics allow.

A subagent's role or delegated task must not weaken the parent-independent runtime contracts. Specialized tool registries remain authoritative even when prompt text shares a common prefix.

Do not copy the complete parent conversation into a child merely for convenience. Delegation should pass the bounded context necessary for the child task. Child findings return to the parent as context; the parent still owns integration and final verification.

Prompt ABI v9 moves normal child-result collection out of model-driven polling. The runtime observes versioned result events, deduplicates them per parent turn, and injects completed child results as ephemeral runtime context. `wait`, `get`, and `list` remain lifecycle inspection capabilities, but the managed prompt does not prescribe them for normal result collection. Delegated work blocks completion by default; `optional=true` is reserved for speculative work that may be integrated if ready but must not delay the parent. The runtime keeps optional work active for safe tentative-output buffering and cancels any still-live optional child when the parent commits. `depends_on` expresses a dependency on already-spawned children in the same parent turn; the runtime waits for those dependencies and only starts the child after all complete successfully, so the model must not poll dependency state.

Prompt ABI v10 adds a structured child-result contract. Subagents end their final response with a `<proton-subagent-result>` JSON envelope containing a concise `conclusion`, optional `findings`, and optional `blockers`. Finding evidence references are accepted only when they match successful runtime-observed tool evidence from that child. Malformed or unsupported structured output falls back to the child's plain-text conclusion, so provider formatting quirks cannot make the delegated run fail. `changed_targets` and verification state remain runtime-derived rather than model-asserted.

Prompt ABI v11 tightens workspace discovery discipline. `read` is for known artifacts; the managed prompt no longer advertises `ls`, `find`, or `grep` when those capabilities are absent, and a `not_found` result for a guessed path must trigger discovery rather than an unchanged retry. Host-side `discover_resource` recovery may attach bounded parent-directory evidence while preserving the original failure.

Prompt ABI v12 promotes Active Goal from a compaction-stability hint to an execution contract. An active goal is treated as the persistent session objective, implementation goals require concrete inspect/modify/verify progress, and the objective remains in force until completed, blocked, changed, or cleared.

Runtime-delivered child content is untrusted evidence, not instruction material. It is appended after the stable managed system prompt and is not persisted as synthetic user conversation history, preserving the system-prefix cache boundary while keeping instruction hierarchy explicit.

The runtime context uses a structured per-child payload with `status`, `conclusion`, validated `findings`, `verification`, `evidence`, `changed_targets`, and bounded `blockers`. Failed or canceled delegated work therefore reaches Universal as explicit state instead of an empty summary.

## Tests

Cache-sensitive behavior is covered primarily in `internal/engine/prompt/assembly_test.go` and prompt/turn behavior tests. Important regressions include:

- deterministic section ordering;
- repeated rendering is byte-stable;
- equivalent tool sets render equivalent guidance regardless of input order;
- workspace, project instructions, skills, role, and active-goal changes preserve the expected earlier prefix;
- root/subagent identities and capability-conditioned contracts remain behaviorally correct;
- the turn engine publishes the managed prompt as the model-facing system message.

When adding or moving a dynamic section, add a divergence-boundary regression test rather than relying only on substring assertions.
