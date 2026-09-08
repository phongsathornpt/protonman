package agent

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/phongsathornpt/protonman/internal/base/contextutil"
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
	if c.closed.Load() {
		close(ch)
		c.eventMu.Unlock()
		return ch, func() {}
	}
	c.subscribers[id] = ch
	c.eventMu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			c.eventMu.Lock()
			if existing, ok := c.subscribers[id]; ok {
				delete(c.subscribers, id)
				close(existing)
			}
			c.eventMu.Unlock()
		})
	}
}

func (c *Coordinator) closeSubscribers() {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	for id, ch := range c.subscribers {
		close(ch)
		delete(c.subscribers, id)
	}
}

func (c *Coordinator) broadcast(ev Event) {
	c.eventMu.RLock()
	defer c.eventMu.RUnlock()
	for _, ch := range c.subscribers {
		enqueueLifecycleEvent(ch, ev)
	}
}

func (c *Coordinator) emit(_ context.Context, ev Event) {
	c.recordActivity(ev)
	c.broadcast(ev)
	if c.eventSink == nil {
		return
	}
	enqueueLifecycleEvent(c.eventQueue, ev)
}

func (c *Coordinator) recordActivity(ev Event) {
	if !terminalLifecycleEvent(ev.Kind) {
		return
	}
	c.activityMu.Lock()
	c.activitySeq++
	c.activityEvent = ev
	close(c.activityNotify)
	c.activityNotify = make(chan struct{})
	c.activityMu.Unlock()
}

func enqueueLifecycleEvent(ch chan Event, ev Event) {
	select {
	case ch <- ev:
		return
	default:
	}
	if !terminalLifecycleEvent(ev.Kind) {
		return
	}
	// Terminal transitions are state-significant. If a bounded queue is full,
	// evict one older wakeup so completion/failure is not silently lost.
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- ev:
	default:
	}
}

func terminalLifecycleEvent(kind EventKind) bool {
	return kind == EventAgentCompleted || kind == EventAgentFailed
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
