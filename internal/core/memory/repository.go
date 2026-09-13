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

// Repository persists durable memory indexes. Implementations must keep global
// and workspace scopes physically and logically isolated.
type Repository interface {
	Load(context.Context, Scope, string) ([]Entry, error)
	Replace(context.Context, Scope, string, []Entry) error
	RecordUsage(context.Context, []UsageRef, time.Time) error
}
