// Package runtimeui projects mutable TUI runtime signals into one deterministic
// presentation state. It deliberately owns wording/priority, while the runtime
// package remains responsible for collecting raw execution signals.
package runtimeui

import (
	"fmt"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Phase is the user-visible lifecycle state of the active TUI operation.
type Phase uint8

const (
	PhaseIdle Phase = iota
	PhaseWorking
	PhaseStreaming
	PhaseToolRunning
	PhaseDelegating
	PhaseRetryWaiting
	PhaseWaitingForInput
	PhaseCanceling
)

func (p Phase) String() string {
	switch p {
	case PhaseWorking:
		return "working"
	case PhaseStreaming:
		return "streaming"
	case PhaseToolRunning:
		return "tool"
	case PhaseDelegating:
		return "delegating"
	case PhaseRetryWaiting:
		return "retry"
	case PhaseWaitingForInput:
		return "waiting-input"
	case PhaseCanceling:
		return "canceling"
	default:
		return "idle"
	}
}

// Input contains runtime facts already known by the TUI. The projection does
// not mutate execution state and therefore cannot drift from the turn engine.
type Input struct {
	Busy              bool
	PermissionPending bool
	Canceling         bool
	Streaming         bool
	Retry             sdk.RetryEvent
	RunningTool       string
	ExplicitActivity  string
	FallbackActivity  string
	AgentActivity     string
	ActiveAgents      int
	ToolCalls         int
	StartedAt         time.Time
	Now               time.Time
}

// State is the normalized status-bar model. Meta values contain no separators
// so renderers can degrade or reorder them without parsing presentation text.
type State struct {
	Phase    Phase
	Activity string
	Meta     []string
}

func (s State) MetaText() string {
	return strings.Join(s.Meta, " · ")
}

// Project applies the single status priority used by the TUI. Blocking user
// input outranks execution, then cancellation/retry, then delegated/tool work,
// then streaming and generic work.
func Project(input Input) State {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}

	if input.PermissionPending {
		state := State{Phase: PhaseWaitingForInput, Activity: "action required", Meta: []string{"permission"}}
		return appendElapsed(state, input.Busy, input.StartedAt, now)
	}
	if !input.Busy {
		return State{Phase: PhaseIdle}
	}

	if input.Canceling {
		activity := "canceling"
		state := State{Phase: PhaseCanceling, Activity: activity}
		if input.ActiveAgents > 0 {
			if strings.TrimSpace(input.AgentActivity) != "" {
				state.Activity = strings.TrimSpace(input.AgentActivity)
			}
			state.Meta = append(state.Meta, agentCount(input.ActiveAgents))
		}
		return appendElapsed(state, true, input.StartedAt, now)
	}

	if activity, retryMeta, ok := RetryStatus(input.Retry, now); ok {
		state := State{Phase: PhaseRetryWaiting, Activity: activity, Meta: retryMeta}
		return appendElapsed(state, true, input.StartedAt, now)
	}

	if input.ActiveAgents > 0 {
		activity := strings.TrimSpace(input.AgentActivity)
		if activity == "" {
			activity = "delegating"
		}
		state := State{Phase: PhaseDelegating, Activity: activity, Meta: []string{agentCount(input.ActiveAgents)}}
		return appendElapsed(state, true, input.StartedAt, now)
	}

	explicit := strings.TrimSpace(input.ExplicitActivity)
	runningTool := strings.TrimSpace(input.RunningTool)
	if runningTool != "" {
		activity := runningTool
		if explicit != "" && explicit != "ready" {
			activity = explicit
		}
		state := State{Phase: PhaseToolRunning, Activity: activity}
		state.Meta = appendToolCount(state.Meta, input.ToolCalls)
		return appendElapsed(state, true, input.StartedAt, now)
	}

	if input.Streaming {
		state := State{Phase: PhaseStreaming, Activity: "streaming response"}
		state.Meta = appendToolCount(state.Meta, input.ToolCalls)
		return appendElapsed(state, true, input.StartedAt, now)
	}

	activity := explicit
	if activity == "" || activity == "ready" {
		activity = strings.TrimSpace(input.FallbackActivity)
	}
	if activity == "" {
		activity = "working"
	}
	state := State{Phase: PhaseWorking, Activity: activity}
	state.Meta = appendToolCount(state.Meta, input.ToolCalls)
	return appendElapsed(state, true, input.StartedAt, now)
}

func appendToolCount(meta []string, count int) []string {
	if count <= 0 {
		return meta
	}
	label := "tool"
	if count != 1 {
		label = "tools"
	}
	return append(meta, fmt.Sprintf("%d %s", count, label))
}

func appendElapsed(state State, active bool, startedAt, now time.Time) State {
	if !active || startedAt.IsZero() || now.Before(startedAt) {
		return state
	}
	duration := now.Sub(startedAt)
	if duration < time.Second {
		state.Meta = append(state.Meta, "0s")
		return state
	}
	state.Meta = append(state.Meta, duration.Truncate(time.Second).String())
	return state
}

func agentCount(count int) string {
	label := "agent"
	if count != 1 {
		label = "agents"
	}
	return fmt.Sprintf("%d %s", count, label)
}

// RetryStatus normalizes retry countdown and reason copy for every TUI surface.
func RetryStatus(retry sdk.RetryEvent, now time.Time) (string, []string, bool) {
	if retry.Attempt <= 0 || retry.RetryAt.IsZero() {
		return "", nil, false
	}
	remaining := retry.RetryAt.Sub(now)
	wait := "now"
	if remaining > 0 {
		if remaining < time.Second {
			wait = "<1s"
		} else {
			seconds := int((remaining + time.Second - 1) / time.Second)
			wait = fmt.Sprintf("%ds", seconds)
		}
	}
	activity := "retrying " + wait
	if retry.Phase == sdk.RetryPhaseCooldown {
		if wait == "now" {
			activity = "cooldown complete"
		} else {
			activity = "cooling down " + wait
		}
	} else if wait != "now" {
		activity = "retrying in " + wait
	}
	attempt := fmt.Sprintf("retry %d", retry.Attempt)
	if retry.MaxRetries > 0 {
		attempt += fmt.Sprintf("/%d", retry.MaxRetries)
	}
	meta := []string{attempt}
	if reason := retryReasonLabel(retry.Reason); reason != "" {
		meta = append(meta, reason)
	}
	return activity, meta, true
}

func retryReasonLabel(reason string) string {
	switch strings.TrimSpace(reason) {
	case "incomplete_stream":
		return "stream interrupted"
	case "first_event_timeout":
		return "provider slow"
	case "idle_event_timeout":
		return "stream stalled"
	case "max_stream_duration":
		return "stream limit"
	case "rate_limit":
		return "rate limited"
	case "overloaded":
		return "provider busy"
	case "transport":
		return "connection interrupted"
	}
	return strings.ReplaceAll(strings.TrimSpace(reason), "_", " ")
}
