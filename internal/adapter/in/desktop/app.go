//go:build desktop

// Package desktop contains the framework adapter for the Wails desktop
// migration. Runtime and ACP behavior belongs in the controller; this package
// only exposes the bounded API consumed by the React frontend.
package desktop

import (
	"context"
	"sync"
)

// Snapshot is the initial Wails frontend contract. It is intentionally small
// until the existing desktop controller responsibilities are moved behind this
// boundary.
type Snapshot struct {
	Status string `json:"status"`
}

// Controller owns framework-independent desktop state and event subscribers.
// ACP supervision will be moved here in the next migration slice.
type Controller struct {
	mu        sync.RWMutex
	state     Snapshot
	subs      map[chan Snapshot]struct{}
	closeOnce sync.Once
	closed    chan struct{}
}

func NewController() *Controller {
	return &Controller{
		state:  Snapshot{Status: "Wails migration shell"},
		subs:   make(map[chan Snapshot]struct{}),
		closed: make(chan struct{}),
	}
}

func (c *Controller) Snapshot(context.Context) Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Controller) Subscribe(ctx context.Context) (<-chan Snapshot, func()) {
	updates := make(chan Snapshot, 8)

	c.mu.Lock()
	select {
	case <-c.closed:
		close(updates)
	default:
		c.subs[updates] = struct{}{}
	}
	c.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			c.mu.Lock()
			if _, ok := c.subs[updates]; ok {
				delete(c.subs, updates)
				close(updates)
			}
			c.mu.Unlock()
		})
	}

	go func() {
		select {
		case <-ctx.Done():
			unsubscribe()
		case <-c.closed:
		}
	}()

	return updates, unsubscribe
}

func (c *Controller) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.mu.Lock()
		for updates := range c.subs {
			close(updates)
			delete(c.subs, updates)
		}
		c.mu.Unlock()
	})
}

// SetStatus is temporary migration plumbing used by the Wails smoke shell and
// its controller tests. ACP-backed state will replace it as responsibilities
// move out of the legacy desktop application.
func (c *Controller) SetStatus(status string) {
	c.mu.Lock()
	c.state.Status = status
	state := c.state
	for updates := range c.subs {
		select {
		case updates <- state:
		default:
		}
	}
	c.mu.Unlock()
}
