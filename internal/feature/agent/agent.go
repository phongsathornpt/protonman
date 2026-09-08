// Package agent coordinates specialized subagents running concurrently
// in background goroutines with capability scoping and context cancellation.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/engine/turn"
)

// Profile classifies Proton's primary and specialized engineering attributes.
type Profile string

const (
	// ProfileUniversal is the adaptive primary software engineering orchestrator.
	ProfileUniversal Profile = "universal"
	// ProfileStrength executes substantial implementation, fixes, and refactors.
	ProfileStrength Profile = "strength"
	// ProfileAgility performs fast, bounded, read-only exploration and tracing.
	ProfileAgility Profile = "agility"
	// ProfileIntelligence handles deep reasoning, difficult debugging, and high-risk engineering work.
	ProfileIntelligence Profile = "intelligence"
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
	case "pow", "worker":
		p = ProfileStrength
	case "int", "explorer", "reviewer":
		p = ProfileAgility
	case "dex":
		p = ProfileIntelligence
	}
	if !p.Valid() {
		return "", fmt.Errorf("unknown agent profile %q: supported profiles are %s", raw, ProfileList(", "))
	}
	return p, nil
}

// IsSubagent reports whether the profile may be delegated by Universal.
func (p Profile) IsSubagent() bool {
	switch p {
	case ProfileStrength, ProfileAgility, ProfileIntelligence:
		return true
	default:
		return false
	}
}

// ParseSubagentProfile validates a delegated specialized attribute.
func ParseSubagentProfile(raw string) (Profile, error) {
	p, err := ParseProfile(raw)
	if err != nil {
		return "", err
	}
	if !p.IsSubagent() {
		return "", fmt.Errorf("profile %q cannot be delegated: supported subagents are %s", raw, SubagentProfileList(", "))
	}
	return p, nil
}

// ShortLabel returns the compact Dota-style attribute label used by the TUI.
func (p Profile) ShortLabel() string {
	switch p {
	case ProfileUniversal:
		return "UNI"
	case ProfileStrength:
		return "STR"
	case ProfileAgility:
		return "AGI"
	case ProfileIntelligence:
		return "INT"
	default:
		return "AGENT"
	}
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
	if !r.Profile.IsSubagent() {
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

// EvidenceRef identifies a successful tool observation made by a subagent.
type EvidenceRef struct {
	Tool   string `json:"tool"`
	Target string `json:"target,omitempty"`
}

// Result is the bounded final output returned from a subagent to its caller.
type Result struct {
	AgentID        string                 `json:"agent_id"`
	Profile        Profile                `json:"profile"`
	Provider       string                 `json:"provider,omitempty"`
	Model          string                 `json:"model,omitempty"`
	Summary        string                 `json:"summary"`
	Rounds         int                    `json:"rounds"`
	Verification   turn.VerificationState `json:"verification"`
	Evidence       []EvidenceRef          `json:"evidence"`
	ChangedTargets []string               `json:"changed_targets"`
	QueueDuration  time.Duration          `json:"queue_duration"`
	Duration       time.Duration          `json:"duration"`
	TotalDuration  time.Duration          `json:"total_duration"`
	Err            error                  `json:"-"`
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
	Call          *tool.Call    `json:"call,omitempty"`
	QueueDuration time.Duration `json:"queue_duration,omitempty"`
	Duration      time.Duration `json:"duration,omitempty"`
	TotalDuration time.Duration `json:"total_duration,omitempty"`
	Err           error         `json:"-"`
}

// EventSink receives lifecycle events emitted during subagent runs.
type EventSink func(context.Context, Event) error
