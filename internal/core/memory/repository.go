package memory

import (
	"context"
	"time"
)

// UsageRef identifies one durable memory entry without leaking persistence paths
// into the application or engine layers.
type UsageRef struct {
	Scope        Scope
	WorkspaceKey string
	ID           string
}

// UpdateFunc transforms one scope index while the repository owns its write lock.
type UpdateFunc func([]Entry) ([]Entry, error)

// RevisionSource is an optional Repository capability. Implementations report a
// value that changes on every index write, so a consumer that caches a read can
// detect a correction such as Forget and stop serving the stale projection.
// Repositories without this capability are treated as permanently unversioned.
type RevisionSource interface {
	Revision() uint64
}

// Repository persists durable memory indexes. Implementations must keep global
// and workspace scopes physically and logically isolated.
type Repository interface {
	Load(context.Context, Scope, string) ([]Entry, error)
	Replace(context.Context, Scope, string, []Entry) error
	Update(context.Context, Scope, string, UpdateFunc) error
	// Forget removes the identified entries from one scope index and reports how
	// many were actually removed. Unknown IDs are ignored so a concurrent prune
	// or repeated request is not an error.
	Forget(context.Context, Scope, string, []string) (int, error)
	RecordUsage(context.Context, []UsageRef, time.Time) error
	ProcessedRevision(context.Context, string) (uint64, bool, error)
	MarkProcessed(context.Context, string, uint64) error
}
