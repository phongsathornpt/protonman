package todo

import "context"

// Repository is the shared task-state boundary used by tools and adapters.
type Repository interface {
	Snapshot() Snapshot
	CompareAndReplace(context.Context, uint64, []Item) (Snapshot, error)
}

// ReloadableRepository can refresh its in-memory projection from durable state.
type ReloadableRepository interface {
	Repository
	Reload(context.Context) (Snapshot, error)
}

// GoalBoundRepository binds durable task state to the active session goal.
// Rebinding to a different non-empty goal supersedes tasks from the previous
// goal while preserving optimistic-concurrency revision semantics.
type GoalBoundRepository interface {
	Repository
	BindGoal(context.Context, string) (Snapshot, bool, error)
}

// PatchRepository atomically validates and applies a patch against one revision.
// Implementations with durable backing should perform read, compare, patch, and write
// inside the same critical section.
type PatchRepository interface {
	Repository
	CompareAndPatch(context.Context, uint64, []Operation) (before Snapshot, after Snapshot, err error)
}
