package agent

import (
	"context"
	"fmt"
	"time"
)

// LifecycleEventStore durably records and replays lifecycle facts per session.
type LifecycleEventStore interface {
	AppendLifecycleEvent(context.Context, LifecycleEvent) error
	LoadLifecycleEvents(context.Context, string) ([]LifecycleEvent, error)
}

// LifecycleEventKind identifies one durable lifecycle transition.
type LifecycleEventKind string

const (
	LifecycleAgentQueued          LifecycleEventKind = "agent_queued"
	LifecycleAgentStarted         LifecycleEventKind = "agent_started"
	LifecycleAgentCancelRequested LifecycleEventKind = "agent_cancel_requested"
	LifecycleAgentCompleted       LifecycleEventKind = "agent_completed"
	LifecycleAgentFailed          LifecycleEventKind = "agent_failed"
	LifecycleAgentCanceled        LifecycleEventKind = "agent_canceled"
	LifecycleAgentInterrupted     LifecycleEventKind = "agent_interrupted"
	LifecycleAgentResumeRequested LifecycleEventKind = "agent_resume_requested"
	LifecycleAgentResumed         LifecycleEventKind = "agent_resumed"
)

// LifecycleEvent is the domain fact used to derive one AgentStatus projection.
type LifecycleEvent struct {
	Kind        LifecycleEventKind `json:"kind"`
	Version     uint64             `json:"version"`
	At          time.Time          `json:"at"`
	SessionID   string             `json:"session_id,omitempty"`
	ParentID    string             `json:"parent_id,omitempty"`
	AgentID     string             `json:"agent_id"`
	Profile     Profile            `json:"profile"`
	Task        string             `json:"task,omitempty"`
	Provider    string             `json:"provider,omitempty"`
	Model       string             `json:"model,omitempty"`
	Reason      string             `json:"reason,omitempty"`
	ResumedFrom string             `json:"resumed_from,omitempty"`
	ResumedAs   string             `json:"resumed_as,omitempty"`
	Request     *Request           `json:"request,omitempty"`
	Result      *Result            `json:"result,omitempty"`
	Error       string             `json:"error,omitempty"`
}

func (e LifecycleEvent) targetState() (State, error) {
	switch e.Kind {
	case LifecycleAgentQueued:
		return StateQueued, nil
	case LifecycleAgentStarted:
		return StateRunning, nil
	case LifecycleAgentCancelRequested:
		return StateCanceling, nil
	case LifecycleAgentCompleted:
		return StateCompleted, nil
	case LifecycleAgentFailed:
		return StateFailed, nil
	case LifecycleAgentCanceled:
		return StateCanceled, nil
	case LifecycleAgentInterrupted:
		return StateInterrupted, nil
	case LifecycleAgentResumeRequested:
		return StateResuming, nil
	case LifecycleAgentResumed:
		return StateResumed, nil
	default:
		return "", fmt.Errorf("unknown lifecycle event kind %q", e.Kind)
	}
}

func nextLifecycleEvent(status AgentStatus, kind LifecycleEventKind, at time.Time, reason string) LifecycleEvent {
	return LifecycleEvent{
		Kind: kind, Version: status.Version + 1, At: at,
		SessionID: status.SessionID, ParentID: status.ParentID, AgentID: status.ID,
		Profile: status.Profile, Task: status.Task, Provider: status.Provider, Model: status.Model,
		Reason: reason, ResumedFrom: status.ResumedFrom, ResumedAs: status.ResumedAs,
	}
}
func applyLifecycleEvent(status AgentStatus, event LifecycleEvent) (AgentStatus, error) {
	if event.Version == 0 || event.Version != status.Version+1 {
		return status, fmt.Errorf("lifecycle version conflict: current=%d event=%d", status.Version, event.Version)
	}
	if status.ID != "" && status.ID != event.AgentID {
		return status, fmt.Errorf("lifecycle agent mismatch: %q != %q", status.ID, event.AgentID)
	}
	if status.SessionID != "" && status.SessionID != event.SessionID {
		return status, fmt.Errorf("lifecycle session mismatch: %q != %q", status.SessionID, event.SessionID)
	}
	target, err := event.targetState()
	if err != nil {
		return status, err
	}
	if status.State == "" {
		if target != StateQueued {
			return status, &StateTransitionError{From: status.State, To: target}
		}
		status = AgentStatus{
			SessionID: event.SessionID, ID: event.AgentID, ParentID: event.ParentID,
			Profile: event.Profile, Provider: event.Provider, Model: event.Model, Task: event.Task,
			State: StateQueued, StartTime: event.At, ResumedFrom: event.ResumedFrom,
		}
	} else {
		status, err = transitionStatus(status, target, event.At, event.Reason)
		if err != nil {
			return status, err
		}
	}
	status.Version = event.Version
	if event.ResumedFrom != "" {
		status.ResumedFrom = event.ResumedFrom
	}
	if event.ResumedAs != "" {
		status.ResumedAs = event.ResumedAs
	}
	return status, nil
}
func applyEntryLifecycleEvent(entry *agentEntry, event LifecycleEvent) error {
	status, err := applyLifecycleEvent(entry.status, event)
	if err != nil {
		return err
	}
	entry.status = status
	return nil
}
