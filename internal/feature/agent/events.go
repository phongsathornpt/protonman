package agent

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/contextutil"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

const (
	maxActivityMailboxEvents = 128
	maxActivityMailboxes     = 256
	activityMailboxTTL       = 30 * time.Minute
)

type activityRecord struct {
	seq   uint64
	event Event
}

type activityMailbox struct {
	nextSeq   uint64
	seen      uint64
	events    []activityRecord
	notify    chan struct{}
	updatedAt time.Time
}

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

func (c *Coordinator) emit(ctx context.Context, ev Event) {
	c.recordActivity(ev)
	switch ev.Kind {
	case EventAgentCompleted:
		c.observeMetric(ctx, MetricEvent{Kind: MetricCompleted, SessionID: ev.SessionID, AgentID: ev.AgentID, ParentID: ev.ParentID, Profile: ev.Profile})
	case EventAgentFailed:
		kind := MetricFailed
		if status, ok := c.Get(ev.AgentID); ok && status.State == StateCanceled {
			kind = MetricCanceled
		}
		c.observeMetric(ctx, MetricEvent{Kind: kind, SessionID: ev.SessionID, AgentID: ev.AgentID, ParentID: ev.ParentID, Profile: ev.Profile})
	}
	c.broadcast(ev)
	if c.eventSink == nil || ev.Kind == EventAgentProgress {
		return
	}
	enqueueLifecycleEvent(c.eventQueue, ev)
}

func (c *Coordinator) recordActivity(ev Event) {
	if !terminalLifecycleEvent(ev.Kind) {
		return
	}
	ev = compactActivityEvent(ev)
	c.activityMu.Lock()
	c.recordActivityLocked("", ev)
	ref := TurnRef{SessionID: ev.SessionID, TurnID: ev.ParentID}.normalized()
	if ref.SessionID != "" || ref.TurnID != "" {
		c.recordActivityLocked(activityScopeKey(ref), ev)
	}
	c.activityMu.Unlock()
	c.pruneActivityMailboxes(time.Now())
}

func compactActivityEvent(ev Event) Event {
	ev.Message = truncatePersistentText(ev.Message, runtimepolicy.AgentActivityMessageBytes)
	if ev.Call != nil {
		call := *ev.Call
		call.Arguments = nil
		ev.Call = &call
	}
	if ev.Err != nil {
		message := truncatePersistentText(ev.Err.Error(), runtimepolicy.AgentActivityErrorBytes)
		if message != "" {
			ev.Err = errors.New(message)
		} else {
			ev.Err = nil
		}
	}
	return ev
}

func (c *Coordinator) recordActivityLocked(scope string, ev Event) {
	mailbox := c.activityMailboxes[scope]
	if mailbox == nil {
		mailbox = &activityMailbox{notify: make(chan struct{})}
		c.activityMailboxes[scope] = mailbox
	}
	mailbox.updatedAt = time.Now()
	mailbox.nextSeq++
	mailbox.events = append(mailbox.events, activityRecord{seq: mailbox.nextSeq, event: ev})
	if len(mailbox.events) > maxActivityMailboxEvents {
		drop := len(mailbox.events) - maxActivityMailboxEvents
		mailbox.events = append([]activityRecord(nil), mailbox.events[drop:]...)
	}
	close(mailbox.notify)
	mailbox.notify = make(chan struct{})
}

func (c *Coordinator) pruneActivityMailboxes(now time.Time) {
	if c == nil {
		return
	}
	active := make(map[string]struct{})
	c.agentsMu.RLock()
	for _, entry := range c.agents {
		if entry.status.State.Terminal() {
			continue
		}
		active[activityScopeKey(TurnRef{SessionID: entry.status.SessionID, TurnID: entry.status.ParentID})] = struct{}{}
	}
	c.agentsMu.RUnlock()

	c.activityMu.Lock()
	defer c.activityMu.Unlock()
	type candidate struct {
		scope string
		at    time.Time
	}
	candidates := make([]candidate, 0, len(c.activityMailboxes))
	for scope, mailbox := range c.activityMailboxes {
		if scope == "" {
			continue
		}
		if _, live := active[scope]; live {
			continue
		}
		if !mailbox.updatedAt.IsZero() && now.Sub(mailbox.updatedAt) >= activityMailboxTTL {
			delete(c.activityMailboxes, scope)
			continue
		}
		candidates = append(candidates, candidate{scope: scope, at: mailbox.updatedAt})
	}
	over := len(c.activityMailboxes) - maxActivityMailboxes
	if over <= 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].at.Before(candidates[j].at) })
	for i := 0; i < over && i < len(candidates); i++ {
		delete(c.activityMailboxes, candidates[i].scope)
	}
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
