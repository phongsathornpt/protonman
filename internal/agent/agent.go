// Package agent coordinates specialized subagents running concurrently
// in background goroutines with capability scoping and context cancellation.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/turn"
)

// Profile classifies the role and capability scope of a subagent.
type Profile string

const (
	// ProfilePOW executes concrete implementation, fixes, and refactors.
	ProfilePOW Profile = "pow"
	// ProfileINT investigates, traces, researches, and reviews without mutating the workspace.
	ProfileINT Profile = "int"
	// ProfileDEX handles complex design, difficult debugging, and high-risk engineering work.
	ProfileDEX Profile = "dex"
)

// Valid reports whether the profile is recognized.
func (p Profile) Valid() bool {
	_, ok := SpecForProfile(p)
	return ok
}

// IsMutating reports whether the profile is allowed to mutate workspace files or run shell commands.
func (p Profile) IsMutating() bool {
	spec, ok := SpecForProfile(p)
	return ok && spec.Mutating
}

// ParseProfile converts a raw string into a validated Profile.
func ParseProfile(raw string) (Profile, error) {
	p := Profile(strings.TrimSpace(strings.ToLower(raw)))
	switch p {
	case "explorer", "reviewer":
		p = ProfileINT
	case "worker":
		p = ProfilePOW
	}
	if !p.Valid() {
		return "", fmt.Errorf("unknown agent profile %q: supported profiles are %s", raw, ProfileList(", "))
	}
	return p, nil
}

// State describes the lifecycle of a persistent subagent.
type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateCanceling State = "canceling"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCanceled  State = "canceled"
)

func (s State) Terminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCanceled
}

// Handle identifies a spawned subagent without coupling its lifetime to a caller wait.
type Handle struct {
	ID      string  `json:"agent_id"`
	Profile Profile `json:"profile"`
}

// WaitResult reports the current state after a bounded wait.
type WaitResult struct {
	State  State   `json:"state"`
	Result *Result `json:"result,omitempty"`
}

// Request is the invocation payload for a delegated subagent.
type Request struct {
	ID           string        `json:"id,omitempty"`
	ParentID     string        `json:"parent_id,omitempty"`
	Profile      Profile       `json:"profile"`
	Task         string        `json:"task"`
	Context      string        `json:"context,omitempty"`
	Timeout      time.Duration `json:"timeout,omitempty"`
	QueueTimeout time.Duration `json:"queue_timeout,omitempty"`
}

// Validate checks request invariants before dispatch.
func (r Request) Validate() error {
	if !r.Profile.Valid() {
		return fmt.Errorf("invalid subagent profile %q", r.Profile)
	}
	if strings.TrimSpace(r.Task) == "" {
		return errors.New("subagent task is required")
	}
	if r.Timeout < 0 {
		return errors.New("subagent timeout cannot be negative")
	}
	if r.QueueTimeout < 0 {
		return errors.New("subagent queue timeout cannot be negative")
	}
	return nil
}

// ErrUnverifiedChanges indicates that a strict mutating profile reached a
// successful model completion without empirical verification after its final mutation.
var ErrUnverifiedChanges = errors.New("subagent completed with unverified changes")

// Result is the bounded final output returned from a subagent to its caller.
type Result struct {
	AgentID       string                 `json:"agent_id"`
	Profile       Profile                `json:"profile"`
	Summary       string                 `json:"summary"`
	Rounds        int                    `json:"rounds"`
	Verification  turn.VerificationState `json:"verification"`
	QueueDuration time.Duration          `json:"queue_duration"`
	Duration      time.Duration          `json:"duration"`
	TotalDuration time.Duration          `json:"total_duration"`
	Err           error                  `json:"-"`
}

// EventKind classifies progress notifications from subagent execution.
type EventKind string

const (
	// EventAgentQueued marks a validated subagent waiting for execution capacity.
	EventAgentQueued EventKind = "agent_queued"
	// EventAgentStarted marks the launch of a subagent execution.
	EventAgentStarted EventKind = "agent_started"
	// EventAgentProgress forwards intermediate text or activity updates.
	EventAgentProgress EventKind = "agent_progress"
	// EventAgentCompleted marks successful completion of a subagent run.
	EventAgentCompleted EventKind = "agent_completed"
	// EventAgentFailed marks a terminal failure or cancellation.
	EventAgentFailed EventKind = "agent_failed"
)

// Event is one lifecycle progress event emitted by an executing subagent.
type Event struct {
	Kind          EventKind     `json:"kind"`
	AgentID       string        `json:"agent_id"`
	ParentID      string        `json:"parent_id,omitempty"`
	Profile       Profile       `json:"profile"`
	Message       string        `json:"message,omitempty"`
	QueueDuration time.Duration `json:"queue_duration,omitempty"`
	Duration      time.Duration `json:"duration,omitempty"`
	TotalDuration time.Duration `json:"total_duration,omitempty"`
	Err           error         `json:"-"`
}

// EventSink receives lifecycle events emitted during subagent runs.
type EventSink func(context.Context, Event) error
