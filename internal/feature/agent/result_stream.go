package agent

import (
	"context"
	"errors"
	"sort"
	"time"
)

// EventCursor is a consumer-owned position in a turn-scoped result stream.
type EventCursor uint64

// ResultEventBatch contains result-availability events after a caller cursor.
type ResultEventBatch struct {
	Events    []Event     `json:"events"`
	Cursor    EventCursor `json:"cursor"`
	Truncated bool        `json:"truncated"`
	TimedOut  bool        `json:"timed_out"`
}

type resultEventMailbox struct {
	nextSeq   uint64
	events    []activityRecord
	notify    chan struct{}
	updatedAt time.Time
}

// WaitResultEventsAfter waits for immutable result references after a caller-
// owned cursor. Reading never advances another consumer's position.
func (c *Coordinator) WaitResultEventsAfter(
	ctx context.Context,
	ref TurnRef,
	after EventCursor,
	timeout time.Duration,
) (ResultEventBatch, error) {
	if c == nil {
		return ResultEventBatch{}, ErrCoordinatorClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ref = ref.normalized()
	if timeout <= 0 {
		timeout = c.waitTimeout
	}

	consume := func() (ResultEventBatch, <-chan struct{}) {
		c.resultEventMu.Lock()
		defer c.resultEventMu.Unlock()
		scope := activityScopeKey(ref)
		mailbox := c.resultEventMailboxes[scope]
		if mailbox == nil {
			mailbox = &resultEventMailbox{notify: make(chan struct{}), updatedAt: time.Now()}
			c.resultEventMailboxes[scope] = mailbox
		}
		mailbox.updatedAt = time.Now()
		events, cursor, truncated := resultEventsAfter(mailbox, uint64(after))
		if len(events) == 0 {
			return ResultEventBatch{Events: []Event{}, Cursor: after}, mailbox.notify
		}
		return ResultEventBatch{Events: events, Cursor: EventCursor(cursor), Truncated: truncated}, nil
	}
	batch, notify := consume()
	if len(batch.Events) > 0 {
		return batch, nil
	}

	waitCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	select {
	case <-notify:
		batch, _ := consume()
		return batch, nil
	case <-waitCtx.Done():
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return ResultEventBatch{Events: []Event{}, Cursor: after, TimedOut: true}, nil
		}
		return ResultEventBatch{}, waitCtx.Err()
	}
}

func resultEventsAfter(mailbox *resultEventMailbox, after uint64) ([]Event, uint64, bool) {
	if mailbox == nil || len(mailbox.events) == 0 {
		return []Event{}, after, false
	}
	oldest := mailbox.events[0].seq
	truncated := after+1 < oldest
	events := make([]Event, 0, len(mailbox.events))
	next := after
	for _, record := range mailbox.events {
		if record.seq <= after {
			continue
		}
		events = append(events, record.event)
		next = record.seq
	}
	return events, next, truncated
}

func (c *Coordinator) recordResultEvent(ev Event) {
	if c == nil || ev.Kind != EventAgentResultAvailable {
		return
	}
	c.resultEventMu.Lock()
	c.recordResultEventLocked("", ev)
	ref := TurnRef{SessionID: ev.SessionID, TurnID: ev.ParentID}.normalized()
	if ref.SessionID != "" || ref.TurnID != "" {
		c.recordResultEventLocked(activityScopeKey(ref), ev)
	}
	c.resultEventMu.Unlock()
	c.pruneResultEventMailboxes(time.Now())
}

func (c *Coordinator) recordResultEventLocked(scope string, ev Event) {
	mailbox := c.resultEventMailboxes[scope]
	if mailbox == nil {
		mailbox = &resultEventMailbox{notify: make(chan struct{})}
		c.resultEventMailboxes[scope] = mailbox
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

func (c *Coordinator) pruneResultEventMailboxes(now time.Time) {
	c.resultEventMu.Lock()
	defer c.resultEventMu.Unlock()
	type candidate struct {
		scope string
		at    time.Time
	}
	candidates := make([]candidate, 0, len(c.resultEventMailboxes))
	for scope, mailbox := range c.resultEventMailboxes {
		if scope == "" {
			continue
		}
		if !mailbox.updatedAt.IsZero() && now.Sub(mailbox.updatedAt) >= activityMailboxTTL {
			delete(c.resultEventMailboxes, scope)
			continue
		}
		candidates = append(candidates, candidate{scope: scope, at: mailbox.updatedAt})
	}
	over := len(c.resultEventMailboxes) - maxActivityMailboxes
	if over <= 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].at.Before(candidates[j].at) })
	for i := 0; i < over && i < len(candidates); i++ {
		delete(c.resultEventMailboxes, candidates[i].scope)
	}
}
