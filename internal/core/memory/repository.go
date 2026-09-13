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

// Repository persists durable memory indexes. Implementations must keep global
// and workspace scopes physically and logically isolated.
type Repository interface {
	Load(context.Context, Scope, string) ([]Entry, error)
	Replace(context.Context, Scope, string, []Entry) error
	Update(context.Context, Scope, string, UpdateFunc) error
	RecordUsage(context.Context, []UsageRef, time.Time) error
	ProcessedRevision(context.Context, string) (uint64, bool, error)
	MarkProcessed(context.Context, string, uint64) error
}
