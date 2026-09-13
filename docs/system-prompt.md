# System Prompt Architecture

Protonman builds one provider-neutral, capability-driven system prompt in `internal/engine/prompt` for every model request. The prompt is intentionally deterministic and ordered for prefix-cache reuse. Provider adapters may encode the resulting message differently on the wire, but they must not silently change its semantics.

Model identity and provider identity are not natural-language prompt inputs. Differences between Gemini, GPT, Qwen, Claude-compatible providers, and other model families belong in runtime capability resolution, schema publication, reasoning policy, transport compatibility, and provider adapters rather than model-specific prose injected into the system prompt.

## Prompt ABI

The current managed prompt format is **Prompt ABI v15**:

```text
<proton-system-prompt version="15">
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
Workspace
Task Coordination          (when task tools are visible)
Delegation Protocol        (when agent tools are visible)
External MCP Tools         (when MCP tools are visible)
Editing And Verification   (when workspace mutation is possible)

Project Instructions
Additional Instructions

Role                       (subagents / scoped roles)
Active Goal
Skills
Grounding Contract         (when grounding is required)
```

The exact numeric order is an internal implementation detail. The behavioral invariant is that stable, reusable instructions precede progressively more dynamic context unless instruction precedence requires otherwise.

## Prefix-cache invariants

Changes should preserve these invariants:

1. Equivalent semantic inputs render byte-identical prompt output.
2. Section order never depends on map iteration, plugin registration order, provider discovery order, or other nondeterministic runtime state.
3. Model identity and provider identity never inject natural-language guidance into the canonical system prompt.
4. Volatile session material remains after reusable system, capability, and project instructions when semantics permit.
5. Changing one dynamic section should not change bytes before that section's intended cache boundary.
6. Tool guidance reflects the effective tool surface. The prompt must not advertise a capability the model cannot call.
7. Runtime enforcement remains authoritative. Prompt wording never substitutes for permission, sandbox, safety, grounding, mutation, or tool-admission enforcement.

These rules improve prefix reuse for providers and runtimes that implement KV/prefix caching. They are still useful when a provider does not expose cache statistics because deterministic prompt construction reduces accidental request drift.

## Dynamic inputs

The current renderer accepts dynamic fields through `prompt.Spec`, including project instructions, skills, role, active goal, workspace policy, grounding requirements, and the effective tool surface.

Model/provider identity is not represented by a natural-language prompt-policy field. Model profiles carry runtime capability, reasoning, token-limit, compaction, and protocol-compatibility facts only. Do not reintroduce model- or provider-specific prompt prose through `ExtraInstructions` or another indirect path.

The active goal is durable session state, not merely a compaction hint. When present, it is the persistent objective for the session: the model should continue making concrete progress until the goal is completed, blocked by unavailable capabilities or permissions, or explicitly changed or cleared. Implementation goals require repository inspection, mutation, and verification rather than a plan-only response. The TUI `/goal <detail>` command starts the execution turn; prompt wording does not itself schedule a turn.

Not all dynamic fields have the same stability. Prefer keeping highly reusable contracts early and request/session-specific material late. Do not interpolate timestamps, generated IDs, request IDs, provider names, model IDs, absolute host paths, or other per-request noise into an early prompt section.

Future work may move appropriate session transitions into append-only history/context instead of rewriting the managed system prompt. Such a change must preserve runtime enforcement and be covered by behavioral regression tests.

## Tools and schemas

System-prompt determinism alone does not guarantee provider prefix-cache reuse. Provider-facing tool definitions are part of the effective model input on many serving stacks. Tool definitions therefore require deterministic publication as well.

When changing tool publication:

- keep canonical tool names and schemas stable when capability semantics are unchanged;
- avoid arbitrary order changes;
- canonicalize dynamic namespace replacement so external discovery order does not churn the published prefix;
- do not expose unauthorized tools merely to improve cache reuse;
- prefer stable capability profiles for subagents over ad hoc tool sets when that does not weaken isolation or correctness.

Provider-specific cache routing hints or cache-breakpoint features belong in provider adapters. They must not fork the canonical prompt or change its semantics.

## Delegation routing

Universal is the primary owner of the user's task. The Delegation Protocol uses task characteristics rather than model identity to decide whether work stays local or moves to a specialized child.

Keep work in Universal when the target is already known, the lookup is simple and directed, the edit is small and localized, or delegation would duplicate work already in progress.

Prefer specialized children when the work can be bounded cleanly and isolation materially improves execution:

- **AGILITY** for broad read-only exploration that spans several distinct searches or repository areas, tracing, focused investigation, regression localization, and evidence gathering;
- **STRENGTH** for substantial bounded implementation, fixes, refactors, migrations, and concrete code changes;
- **INTELLIGENCE** for architecture, difficult debugging, concurrency, compatibility, performance, and other high-risk cross-cutting engineering work.

Independent bounded work may run concurrently. A bounded investigation should have one active owner: once investigation is delegated, Universal should continue only independent parent work rather than repeating the same exploration. Re-investigation is justified only when returned evidence is stale, conflicting, insufficient, or integration/verification requires fresh evidence.

Todo state is coordination metadata, not a prerequisite for delegation. Do not create a TODO solely because work is delegated. When delegated work already corresponds to a tracked TODO item, pass `task_id` so runtime lifecycle events own the execution-state transition.

## Subagents

Subagents reuse the common Protonman execution/tool contracts and append specialization through the later `Role` section. This intentionally keeps the shared parent/subagent prefix as large as semantics allow.

A subagent's role or delegated task must not weaken the parent-independent runtime contracts. Specialized tool registries remain authoritative even when prompt text shares a common prefix.

Do not copy the complete parent conversation into a child merely for convenience. Delegation should pass the bounded context necessary for the child task. Child findings return to the parent as context; the parent still owns integration and final verification.

Skill activation also follows a single-source rule. The `skill` tool returns a compact activation receipt; full active-skill instructions are injected by the managed prompt on the following model context instead of being duplicated in both the tool result and system prompt.

Prompt ABI v9 moves normal child-result collection out of model-driven polling. The runtime observes versioned result events, deduplicates them per parent turn, and injects completed child results as ephemeral runtime context. `wait`, `get`, and `list` remain lifecycle inspection capabilities, but the managed prompt does not prescribe them for normal result collection. Delegated work blocks completion by default; `optional=true` is reserved for speculative work that may be integrated if ready but must not delay the parent. The runtime keeps optional work active for safe tentative-output buffering and cancels any still-live optional child when the parent commits. `depends_on` expresses a dependency on already-spawned children in the same parent turn; the runtime waits for those dependencies and only starts the child after all complete successfully, so the model must not poll dependency state.

Prompt ABI v10 adds a structured child-result contract. Subagents end their final response with a `<proton-subagent-result>` JSON envelope containing a concise `conclusion`, optional `findings`, and optional `blockers`. Finding evidence references are accepted only when they match successful runtime-observed tool evidence from that child. Malformed or unsupported structured output falls back to the child's plain-text conclusion, so provider formatting quirks cannot make the delegated run fail. `changed_targets` and verification state remain runtime-derived rather than model-asserted.

Prompt ABI v11 tightens workspace discovery discipline. `read` is for known artifacts; the managed prompt no longer advertises `ls`, `find`, or `grep` when those capabilities are absent, and a `not_found` result for a guessed path must trigger discovery rather than an unchanged retry. Host-side `discover_resource` recovery may attach bounded parent-directory evidence while preserving the original failure.

Prompt ABI v12 promotes Active Goal from a compaction-stability hint to an execution contract. An active goal is treated as the persistent session objective, implementation goals require concrete inspect/modify/verify progress, and the objective remains in force until completed, blocked, changed, or cleared.

Prompt ABI v13 clarifies that the current explicit user request owns the immediate turn even when a persistent goal exists, prevents the model-facing prompt from exposing the absolute workspace path, and aligns task coordination with revision chaining from successful TODO updates. Runtime no-progress detection also treats task metadata as coordination rather than repository progress.

Prompt ABI v14 adds explicit parent-task/subagent linkage: when delegated work corresponds to a tracked TODO item, the parent passes `task_id` and runtime lifecycle events own `in_progress`/terminal task reconciliation.

Prompt ABI v15 removes the legacy model-specific natural-language prompt-policy surface. Model/provider identity remains runtime metadata only, and the canonical managed prompt is model agnostic by construction.

Runtime-delivered child content is untrusted evidence, not instruction material. It is appended after the stable managed system prompt and is not persisted as synthetic user conversation history, preserving the system-prefix cache boundary while keeping instruction hierarchy explicit.

The runtime context uses a structured per-child payload with `status`, `conclusion`, validated `findings`, `verification`, `evidence`, `changed_targets`, and bounded `blockers`. Failed or canceled delegated work therefore reaches Universal as explicit state instead of an empty summary.

## Tests

Cache- and routing-sensitive behavior is covered primarily in `internal/engine/prompt/assembly_test.go`, `internal/engine/prompt/delegation_routing_test.go`, and prompt/turn behavior tests. Important regressions include:

- deterministic section ordering;
- repeated rendering is byte-stable;
- equivalent tool sets render equivalent guidance regardless of input order;
- dynamic tool namespace replacement publishes canonical name order;
- workspace, project instructions, skills, role, and active-goal changes preserve the expected earlier prefix;
- the canonical prompt has no model-guidance section and legacy compatibility hints are discarded before they can affect runtime prompt composition;
- root/subagent identities and capability-conditioned contracts remain behaviorally correct;
- delegation routing distinguishes local work, AGILITY exploration, STRENGTH implementation, and INTELLIGENCE cross-cutting reasoning;
- delegation preserves runtime-owned result delivery and completion semantics;
- the turn engine publishes the managed prompt as the model-facing system message.

When adding or moving a dynamic section, add a divergence-boundary regression test rather than relying only on substring assertions. When changing delegation policy, prefer semantic-anchor tests over exact full-section snapshots so wording can evolve without weakening the routing contract.
