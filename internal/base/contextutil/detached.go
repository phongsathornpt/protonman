package contextutil

import (
	"context"
	"time"
)

// DetachedTimeout preserves parent values while allowing bounded cleanup work
// to outlive parent cancellation.
func DetachedTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
