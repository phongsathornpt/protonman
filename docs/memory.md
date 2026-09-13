# Durable Memory Architecture

Protonman durable memory preserves a small amount of high-signal knowledge from prior root sessions so future turns can reuse stable preferences, repository facts, proven procedures, failure shields, and adopted decisions without replaying old transcripts.

Memory is deliberately separate from conversation retention and compaction. Compaction reduces the current model context to fit a token budget. Durable memory extracts reusable knowledge across sessions. A compacted message is never promoted merely because it survived compaction.

## Design invariants

1. Historical memory is supporting evidence, never instruction authority.
2. The current user request, current repository evidence, permission policy, and runtime contracts outrank memory.
3. Memory may be stale. Drift-prone repository facts and failure notes expire from retrieval according to the central runtime policy.
4. Root sessions own memory extraction. Subagents do not automatically load or write durable memory.
5. Retrieval is bounded and deterministic. V1 uses lexical relevance rather than an embedding/vector dependency.
6. Secret-shaped data is redacted before extraction-model input and again before persistence.
7. Extracted memories require valid persisted message IDs as provenance.
8. Processing is idempotent per persisted session revision.
9. Canonical managed system-prompt bytes do not change because memory changes.

## Filesystem layout

The canonical user-global namespace is resolved through `internal/app/appdirs`:

```text
~/.protonman/memory/
  v1/
    global/
      index.json
    workspaces/
      <workspace-key>/
        index.json
    sources/
      processed.json
```

Global and workspace indexes are physically isolated. Workspace keys and session IDs are validated as safe path segments before they can participate in a persistence path.

Memory files are written through temporary files followed by `fsync` and rename. Scope-level locks serialize read-modify-write index operations. The processed-revision ledger is monotonic so an older worker cannot move a session backwards.

## Memory model

The provider-neutral domain contract lives in `internal/core/memory`.

Scopes:

- `workspace`: repository/project-specific knowledge.
- `global`: cross-project user operating preferences only.

Kinds:

- `preference`
- `repo_fact`
- `procedure`
- `failure`
- `decision`

Each entry carries a stable ID, retrieval key/value, optional keywords, confidence, timestamps, usage metadata, and evidence references back to session ID, session revision, and persisted message IDs.

## Extraction write path

`internal/feature/memory.Extractor` runs asynchronously after the first usable primary model is built. Work is bounded by `runtimepolicy.DurableMemory()`.

The extraction pass:

1. Lists recent sessions for the current workspace.
2. Skips the currently active session.
3. Skips sessions that have not been idle long enough.
4. Skips a session revision already recorded in `processed.json`.
5. Builds a bounded redacted transcript from persisted session messages.
6. Calls the model with no tools and a strict structured JSON extraction contract.
7. Drops low-confidence candidates and candidates without valid source message IDs.
8. Merges accepted candidates transactionally into workspace/global indexes.
9. Marks the session revision processed only after the merge succeeds. A valid no-op extraction is also marked processed.

A failed extraction does not block the interactive model turn. The background pass has its own timeout and will retry a still-unprocessed revision on a later process start.

### Promotion rules

Repository facts, procedures, failures, and decisions are workspace-scoped.

A preference can become global when either:

- the user explicitly stated it and extraction confidence is high; or
- the same inferred preference accumulates evidence across the minimum number of independent sessions configured by `GlobalPromotionMinSessions`.

Promotion removes the equivalent workspace preference so retrieval does not inject duplicate guidance.

## Retrieval read path

`internal/feature/memory.Retriever` loads the current workspace index plus the global preference index. It tokenizes the current user query, scores matching memory deterministically, filters stale facts, and returns a bounded top set.

Ranking favors:

1. lexical matches in memory keys;
2. keyword matches;
3. value matches;
4. workspace-local entries;
5. confidence;
6. previous successful usage.

Selected entries update best-effort usage metadata. Failure to update usage counters never fails the user turn.

## Model-request injection

Memory does not become an `Additional Instructions` section and does not modify the managed system prompt. The primary model factory decorates provider requests after the turn engine has already produced the canonical request.

For a relevant current user query, the decorator inserts one bounded assistant-role historical-evidence message immediately before the current user message:

```text
<proton-memory-context>
Historical memory from prior root sessions follows...
<memory-context>
  ...
</memory-context>
</proton-memory-context>
```

The inserted message exists only in the provider request. It is not appended to turn history, not returned in `turn.Result.Messages`, and therefore not persisted by the session store. Repeated model rounds for the same stable user message reuse the same retrieved context rather than incrementing usage repeatedly.

The primary runtime uses `NewPrimaryModelFactory`. The subagent model resolver intentionally keeps the undecorated base model factory, preserving child isolation.

## Runtime policy

`internal/base/runtimepolicy.DurableMemory()` is the single source of truth for:

- maximum entries injected per turn;
- maximum injected context bytes;
- maximum entries retained per index;
- maximum sessions processed per startup extraction pass;
- maximum extraction transcript bytes;
- idle age and extraction timeout;
- global-preference promotion threshold;
- stale ages for repository facts and failure memories.

Do not duplicate these values in adapters, UI code, or tests.

## Security and trust boundary

Persisted session state already excludes raw tool arguments. Durable memory adds another boundary:

- secret-shaped source content is redacted before the extraction model receives it;
- generated keys, values, and keywords are redacted before they are accepted;
- candidates must cite persisted message IDs from the source session;
- historical markup is escaped before provider-request injection;
- workspace-scoped entries cannot escape their workspace identity;
- memory never substitutes for live verification when a fact can drift.

Memory quality should optimize for future user time saved, not for maximizing the amount of stored text. A clean no-op is preferable to low-signal memory.
