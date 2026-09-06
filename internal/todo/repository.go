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
