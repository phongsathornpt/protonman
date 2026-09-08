package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	PersistentSnapshotVersion = 1
	maxPersistentTaskBytes    = 16 * 1024
	maxPersistentContextBytes = 32 * 1024
)

// PersistentSnapshot is the bounded durable representation of retained agent
// lifecycle state. It contains no model client, context, or cancellation handle.
type PersistentSnapshot struct {
	Version   int               `json:"version"`
	Agents    []PersistentAgent `json:"agents"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// PersistentAgent contains enough state to inspect a prior run and explicitly
// resume interrupted work without serializing process-owned runtime objects.
type PersistentAgent struct {
	Status  AgentStatus `json:"status"`
	Request Request     `json:"request"`
	Result  *Result     `json:"result,omitempty"`
}

// PersistentSnapshot returns a detached copy suitable for durable storage.
func (c *Coordinator) PersistentSnapshot() PersistentSnapshot {
	return c.PersistentSnapshotForSession("")
}

// PersistentSnapshotForSession returns a detached projection for one session.
// An empty session ID preserves the compatibility behavior of returning all agents.
func (c *Coordinator) PersistentSnapshotForSession(sessionID string) PersistentSnapshot {
	if c == nil {
		return PersistentSnapshot{Version: PersistentSnapshotVersion, Agents: []PersistentAgent{}, UpdatedAt: time.Now().UTC()}
	}
	sessionID = strings.TrimSpace(sessionID)
	c.pruneExpired()
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	return c.persistentSnapshotLocked(sessionID)
}

func (c *Coordinator) persistentSnapshotLocked(sessionID string) PersistentSnapshot {
	agents := make([]PersistentAgent, 0, len(c.agents))
	for _, entry := range c.agents {
		if sessionID != "" && entry.status.SessionID != sessionID {
			continue
		}
		status := entry.status
		request := entry.request
		request.Task = truncatePersistentText(request.Task, maxPersistentTaskBytes)
		request.Context = truncatePersistentText(request.Context, maxPersistentContextBytes)
		status.Task = request.Task
		record := PersistentAgent{Status: status, Request: request}
		if entry.status.State.Terminal() {
			result := cloneResult(entry.result)
			record.Result = &result
		}
		agents = append(agents, record)
	}
	return PersistentSnapshot{Version: PersistentSnapshotVersion, Agents: agents, UpdatedAt: time.Now().UTC()}
}

// RestorePersistentSnapshot restores retained lifecycle records. Runs that were
// live in the previous process become interrupted and are never auto-replayed.
func (c *Coordinator) RestorePersistentSnapshot(snapshot PersistentSnapshot) error {
	if c == nil || len(snapshot.Agents) == 0 {
		return nil
	}
	if snapshot.Version != PersistentSnapshotVersion {
		return fmt.Errorf("unsupported agent snapshot version %d", snapshot.Version)
	}

	interruptedEvents := make([]MetricEvent, 0)
	c.agentsMu.Lock()
	if c.closed.Load() {
		c.agentsMu.Unlock()
		return ErrCoordinatorClosed
	}
	for _, record := range snapshot.Agents {
		if strings.TrimSpace(record.Status.ID) == "" || !record.Status.Profile.IsSubagent() {
			continue
		}
		if _, exists := c.agents[record.Status.ID]; exists {
			continue
		}
		status := record.Status
		request := record.Request
		request.SessionID = status.SessionID
		request.ID = status.ID
		request.ParentID = status.ParentID
		request.Profile = status.Profile
		request.Task = status.Task
		interrupted := false
		if !status.State.Terminal() {
			event := nextLifecycleEvent(status, LifecycleAgentInterrupted, time.Now().UTC(), "interrupted by previous process exit")
			if err := c.persistLifecycleEvent(c.rootCtx, event); err != nil {
				c.agentsMu.Unlock()
				return fmt.Errorf("persist restored subagent %q interruption: %w", status.ID, err)
			}
			nextStatus, transitionErr := applyLifecycleEvent(status, event)
			if transitionErr != nil {
				c.agentsMu.Unlock()
				return fmt.Errorf("restore subagent %q: %w", status.ID, transitionErr)
			}
			status = nextStatus
			interrupted = true
		}
		result := Result{SessionID: status.SessionID, AgentID: status.ID, Profile: status.Profile, Provider: status.Provider, Model: status.Model}
		if record.Result != nil {
			result = cloneResult(*record.Result)
		}
		done := make(chan struct{})
		started := make(chan struct{})
		close(done)
		close(started)
		c.agents[status.ID] = &agentEntry{status: status, request: request, result: result, cancel: func() {}, done: done, started: started}
		c.raiseSequenceForID(status.ID)
		if interrupted {
			interruptedEvents = append(interruptedEvents, MetricEvent{Kind: MetricInterrupted, SessionID: status.SessionID, AgentID: status.ID, ParentID: status.ParentID, Profile: status.Profile})
		}
	}
	c.pruneExpiredLocked(time.Now())
	c.agentsMu.Unlock()
	for _, event := range interruptedEvents {
		c.observeMetric(context.Background(), event)
	}
	return nil
}

func cloneResult(result Result) Result {
	result.Evidence = append([]EvidenceRef(nil), result.Evidence...)
	result.ChangedTargets = append([]string(nil), result.ChangedTargets...)
	result.Err = nil
	return result
}

func (c *Coordinator) raiseSequenceForID(id string) {
	parts := strings.Split(id, "-")
	if len(parts) < 2 {
		return
	}
	value, err := strconv.ParseUint(parts[len(parts)-1], 10, 64)
	if err != nil {
		return
	}
	for {
		current := atomic.LoadUint64(&c.seq)
		if current >= value || atomic.CompareAndSwapUint64(&c.seq, current, value) {
			return
		}
	}
}

func truncatePersistentText(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	value = strings.ToValidUTF8(value, "")
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut]
}
