// Package checkpoint defines the provider-neutral checkpoint boundary.
package checkpoint

import "context"

// Store captures files before an edit and restores a captured checkpoint.
type Store interface {
	Capture(ctx context.Context, paths []string) (string, error)
	Restore(ctx context.Context, id string) error
}
