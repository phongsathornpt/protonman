package agent

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/projectTHORN/proton/internal/contextutil"
)

// Subscribe returns a bounded lifecycle stream. Slow subscribers drop events
// rather than blocking agent execution; callers resnapshot coordinator state on
// every delivered event, so events are wakeups rather than the source of truth.
func (c *Coordinator) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan Event, buffer)
	id := atomic.AddUint64(&c.subscriberSeq, 1)
	c.eventMu.Lock()
	c.subscribers[id] = ch
	c.eventMu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			c.eventMu.Lock()
			delete(c.subscribers, id)
			close(ch)
			c.eventMu.Unlock()
		})
	}
}

func (c *Coordinator) broadcast(ev Event) {
	c.eventMu.RLock()
	defer c.eventMu.RUnlock()
	for _, ch := range c.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}
func (c *Coordinator) emit(_ context.Context, ev Event) {
	c.broadcast(ev)
	if c.eventSink == nil {
		return
	}
	select {
	case c.eventQueue <- ev:
	default:
	}
}

func (c *Coordinator) runEventSink() {
	for {
		select {
		case ev := <-c.eventQueue:
			emitCtx, done := contextutil.DetachedTimeout(c.rootCtx, eventEmitTimeout)
			_ = c.eventSink(emitCtx, ev)
			done()
		case <-c.rootCtx.Done():
			return
		}
	}
}
