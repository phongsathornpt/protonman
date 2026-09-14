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
10. Durable-memory decoration must preserve the provider-visible message-role sequence.
11. A root model factory never retrieves memory until it is bound to an active
    workspace, and a session switch rebinds it rather than leaving it on the
    session it was constructed with.
12. Subagent admission builds from the undecorated base factory. A child that
    inherits the root model must not inherit root memory decoration.
13. A wrong or harmful memory can be permanently forgotten. Forgetting is
    authorized from trusted session context, never from a client-supplied
    workspace key, and it takes effect on the next turn.

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

The composition root builds one session-bound root factory
(`NewSessionBoundModelFactory`) and binds it to the active session through
`app.BindRootMemory`. The TUI re-binds on every runner reconfiguration, so a
`/resume` switch moves retrieval and extraction onto the newly active session.
Extraction is claimed once per active session per process; switching sessions
re-arms it, while repeated model builds within one session do not.

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

Session-local load/model failures are logged and skipped so one bad historical session cannot abort the rest of the extraction queue. Failed revisions remain unprocessed and therefore retry on a later pass. Context cancellation and durable-store failures still terminate the pass because continuing would not be safe or useful.

The background pass never blocks the interactive model turn and has its own timeout.

### Promotion and replacement rules

Repository facts, procedures, failures, and decisions are workspace-scoped.

A preference can become global when either:

- the user explicitly stated it and extraction confidence is high; or
- the same inferred preference accumulates evidence across the minimum number of independent sessions configured by `GlobalPromotionMinSessions`.

Promotion removes the equivalent workspace preference so retrieval does not inject duplicate guidance.

For an existing memory ID, a newer value replaces the current value when its confidence is at least as high, or when it is newer and within the bounded replacement-confidence slack while still above the extraction confidence floor. A much lower-confidence candidate cannot overwrite a high-confidence value, but its newer evidence timestamp may still refresh provenance metadata.

## Retrieval read path

`internal/feature/memory.Retriever` loads the current workspace index plus the global preference index. It tokenizes the current user query, filters English stop-words, scores matching memory deterministically, filters stale facts, and returns a bounded top set.

Lexical relevance is evaluated before any workspace, confidence, or usage bonus is applied. This prevents weak common-word matches from accumulating usage and permanently biasing future rankings.

After stale/relevance filtering, entries with the same normalized `(kind, key)` are deduplicated across scopes. A workspace-local entry wins over an equivalent global entry so project-specific guidance cannot be duplicated or diluted by a broader preference.

Ranking favors:

1. lexical matches in memory keys;
2. keyword matches;
3. low-weight value matches;
4. workspace-local entries;
5. confidence;
6. previous retrieval usage.

Value-only overlap is intentionally too weak to qualify unless the lexical evidence is substantial. Selected entries update best-effort usage metadata only after they pass the relevance threshold. Failure to update usage counters never fails the user turn.

The model-request decorator caches a retrieval result per `(workspace, revision, query)`. The repository revision lets a correction such as forget take effect on the next turn instead of being masked by that cache. Repositories that do not implement `RevisionSource` are treated as permanently unversioned, so the cache then persists until the model is rebuilt.

## Model-request injection

Memory does not become an `Additional Instructions` section and does not modify the managed system prompt. The primary model factory decorates provider requests after the turn engine has already produced the canonical request.

For a relevant current user query, the decorator clones the request and prepends one bounded historical-evidence block to the **current user message content**:

```text
<proton-memory-context>
Historical memory from prior root sessions follows...
<memory-context>
  ...
</memory-context>
</proton-memory-context>

<current user content>
```

No additional assistant/user message is inserted. The provider-visible message-role sequence therefore remains unchanged, including on providers that strictly require alternating user/assistant turns.

The decorated content exists only in the provider request. It is not appended to turn history, not returned in `turn.Result.Messages`, and therefore not persisted by the session store. Repeated model rounds for the same stable user message reuse the same retrieved context rather than incrementing usage repeatedly.

A historical message that merely contains the memory marker does not disable future retrieval. Only an already-decorated current user turn suppresses duplicate decoration.

The primary runtime builds root-session models from the session-bound memory
factory. Two boundaries keep children isolated:

- Explicit subagent model overrides are resolved from the undecorated base
  factory in the composition root.
- Profiles without an explicit model override inherit the model installed on the
  coordinator. `app.BuildConversation` installs the undecorated base there, so
  inheritance cannot smuggle memory decoration into a child turn.

All other callers may build directly from the application model factory and
receive retrieval decoration.

## Runtime policy

`internal/base/runtimepolicy.DurableMemory()` is the single source of truth for:

- maximum entries injected per turn;
- maximum injected context bytes;
- maximum entries retained per index;
- maximum remembered tombstones per index;
- maximum sessions processed per startup extraction pass;
- maximum extraction transcript bytes;
- idle age and extraction timeout;
- global-preference promotion threshold;
- stale ages for repository facts and failure memories.

Do not duplicate these values in adapters, UI code, or tests.

## Correction and forget path

`app.Memories.Forget` is the correction capability for durable memory. It is
deliberately separate from `Inspect`, because inspecting is side-effect free while
forgetting changes future model behavior.

Authority is fail-closed:

- workspace scope requires the caller's trusted workspace key; a blank key is
  rejected rather than widened to every workspace;
- global scope must be requested explicitly and cannot carry a workspace key;
- the ACP method resolves the workspace key from session state, so a client
  cannot retarget a forget at an unrelated workspace;
- one request is bounded to `maxForgetIDs` entries.

`memoryfs.FileStore.Forget` removes entries under the same per-scope write lock
and atomic index replacement used by `Replace`/`Update`, so a concurrent
retrieval or extraction merge cannot observe a partial delete. Removing an
unknown ID is not an error, which keeps a repeated request idempotent.

### Why forgetting needs a tombstone

Extraction does not store a memory once; it re-derives an entry's ID
deterministically from `(scope, workspace, kind, key)` and merges by ID on every
pass over a source session. A plain delete would therefore be silently undone the
next time that session was extracted, because the regenerated entry's ID would no
longer be present in the index.

`Forget` consequently records each ID in a bounded `forgotten` list stored in the
same index file. Every write path (`Update`, `Replace`, `RecordUsage`) loads that
list and suppresses tombstoned IDs, and `Load` filters against it as defense in
depth. The tombstone set is persisted, so the correction survives a process
restart, and it is the reason a forgotten memory stays forgotten.

The tombstone set is bounded by `MaxForgottenEntries`. Eviction drops the oldest
forgets first, so the most recent correction is always honored; the bound is
larger than `MaxIndexEntries` because a tombstone is far smaller than the entry it
suppresses.

## Security and trust boundary

Persisted session state already excludes raw tool arguments. Durable memory adds another boundary:

- secret-shaped source content is redacted before the extraction model receives it;
- generated keys, values, and keywords are redacted before they are accepted;
- candidates must cite persisted message IDs from the source session;
- historical markup is escaped before provider-request injection;
- a forget request cannot name a workspace or scope it was not authorized for;
- workspace-scoped entries cannot escape their workspace identity;
- memory never substitutes for live verification when a fact can drift.

Memory quality should optimize for future user time saved, not for maximizing the amount of stored text. A clean no-op is preferable to low-signal memory.
