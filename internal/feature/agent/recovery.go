package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RecoverLifecycle rebuilds one session projection from an optional snapshot and
// subsequent durable lifecycle events. Any run lacking a live runtime becomes
// interrupted and is never replayed automatically.
func (c *Coordinator) RecoverLifecycle(
	ctx context.Context,
	sessionID string,
	snapshot *PersistentSnapshot,
	events []LifecycleEvent,
) error {
	if c == nil {
		return ErrCoordinatorClosed
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("recover subagents: session id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	interruptedMetrics := make([]MetricEvent, 0)
	c.agentsMu.Lock()
	defer func() {
		c.agentsMu.Unlock()
		for _, event := range interruptedMetrics {
			c.observeMetric(context.Background(), event)
		}
	}()
	if c.closed.Load() {
		return ErrCoordinatorClosed
	}
	if snapshot != nil {
		if snapshot.Version != PersistentSnapshotVersion {
			return fmt.Errorf("unsupported agent snapshot version %d", snapshot.Version)
		}
		for _, record := range snapshot.Agents {
			status := record.Status
			if strings.TrimSpace(status.ID) == "" || !status.Profile.IsSubagent() {
				continue
			}
			if status.SessionID == "" {
				status.SessionID = sessionID
			}
			if status.SessionID != sessionID {
				continue
			}
			if status.Version == 0 {
				status.Version = 1
			}
			request := record.Request
			request.SessionID = sessionID
			request.ID = status.ID
			request.ParentID = status.ParentID
			request.Profile = status.Profile
			request.Task = status.Task
			result := Result{SessionID: sessionID, AgentID: status.ID, Profile: status.Profile, Provider: status.Provider, Model: status.Model}
			if record.Result != nil {
				result = cloneResult(*record.Result)
			}
			c.installRecoveredEntry(status, request, result, nil)
		}
	}
	for _, event := range events {
		if event.SessionID != sessionID || strings.TrimSpace(event.AgentID) == "" {
			continue
		}
		entry := c.agents[event.AgentID]
		if entry == nil {
			status, err := applyLifecycleEvent(AgentStatus{}, event)
			if err != nil {
				return fmt.Errorf("replay subagent %q: %w", event.AgentID, err)
			}
			request := requestFromLifecycleEvent(event, status)
			result, resultErr := resultFromLifecycleEvent(event, status)
			c.installRecoveredEntry(status, request, result, resultErr)
			continue
		}
		if event.Version <= entry.status.Version {
			continue
		}
		if err := applyEntryLifecycleEvent(entry, event); err != nil {
			return fmt.Errorf("replay subagent %q: %w", event.AgentID, err)
		}
		if event.Request != nil {
			entry.request = requestFromLifecycleEvent(event, entry.status)
		}
		if event.Result != nil || event.Error != "" {
			entry.result, entry.err = resultFromLifecycleEvent(event, entry.status)
		}
	}
	for _, entry := range c.agents {
		if entry.status.SessionID != sessionID || entry.status.State.Terminal() {
			continue
		}
		event := nextLifecycleEvent(entry.status, LifecycleAgentInterrupted, time.Now().UTC(), "interrupted by previous process exit")
		if err := c.persistLifecycleEvent(ctx, event); err != nil {
			return fmt.Errorf("persist recovered subagent %q interruption: %w", entry.status.ID, err)
		}
		if err := applyEntryLifecycleEvent(entry, event); err != nil {
			return fmt.Errorf("interrupt recovered subagent %q: %w", entry.status.ID, err)
		}
		interruptedMetrics = append(interruptedMetrics, MetricEvent{
			Kind: MetricInterrupted, SessionID: sessionID, AgentID: entry.status.ID,
			ParentID: entry.status.ParentID, Profile: entry.status.Profile,
		})
	}
	c.pruneExpiredLocked(time.Now())
	return nil
}
func (c *Coordinator) installRecoveredEntry(status AgentStatus, request Request, result Result, runErr error) {
	done := make(chan struct{})
	started := make(chan struct{})
	close(done)
	close(started)
	c.agents[status.ID] = &agentEntry{
		status: status, request: request, result: result, err: runErr,
		cancel: func() {}, done: done, started: started,
	}
	c.raiseSequenceForID(status.ID)
}

func requestFromLifecycleEvent(event LifecycleEvent, status AgentStatus) Request {
	if event.Request != nil {
		request := *event.Request
		request.SessionID = status.SessionID
		request.ID = status.ID
		request.ParentID = status.ParentID
		request.Profile = status.Profile
		request.Task = status.Task
		return request
	}
	return Request{
		SessionID: status.SessionID, ID: status.ID, ParentID: status.ParentID,
		Profile: status.Profile, Task: status.Task, Optional: status.Optional, ResumedFrom: status.ResumedFrom,
	}
}
func resultFromLifecycleEvent(event LifecycleEvent, status AgentStatus) (Result, error) {
	result := Result{
		SessionID: status.SessionID, AgentID: status.ID, Profile: status.Profile,
		Provider: status.Provider, Model: status.Model,
	}
	if event.Result != nil {
		result = cloneResult(*event.Result)
		result.SessionID = status.SessionID
		result.AgentID = status.ID
		result.Profile = status.Profile
	}
	var runErr error
	if strings.TrimSpace(event.Error) != "" {
		runErr = errors.New(event.Error)
		result.Err = runErr
	}
	return result, runErr
}
