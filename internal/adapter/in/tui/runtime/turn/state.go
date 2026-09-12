package turn

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Progress tracks execution rounds, tool calls, and retries in a turn.
type Progress struct {
	Round     int
	ToolCalls int
	Retry     sdk.RetryEvent
}

// State tracks the execution state of an active turn or tool call.
type State struct {
	Progress        Progress
	ActiveTurnOwner string
	Busy            bool
	Activity        string
	PendingActivity string
	BusyStarted     time.Time
	Cancel          context.CancelFunc
	Events          <-chan tea.Msg
}

// NewState returns an initialized idle turn state.
func NewState() State {
	return State{Activity: "ready"}
}

// BeginTurn marks state as busy with a new turn owner.
func (s *State) BeginTurn(owner string, started time.Time) {
	if s == nil {
		return
	}
	s.Progress = Progress{}
	s.ActiveTurnOwner = owner
	s.Busy = true
	s.BusyStarted = started
	s.Activity = ""
}

// BindTurn associates cancellation and event streaming channels with the active turn.
func (s *State) BindTurn(cancel context.CancelFunc, events <-chan tea.Msg) {
	if s == nil {
		return
	}
	s.Cancel = cancel
	s.Events = events
}

// FinishTurn marks state as idle and clears turn-specific handles.
func (s *State) FinishTurn() {
	if s == nil {
		return
	}
	s.Busy = false
	s.BusyStarted = time.Time{}
	s.Activity = "ready"
	s.Cancel = nil
	s.Events = nil
	s.ActiveTurnOwner = ""
}

// BeginTool marks state as busy executing a direct tool.
func (s *State) BeginTool(activity string, started time.Time, cancel context.CancelFunc) {
	if s == nil {
		return
	}
	s.Progress = Progress{}
	s.ActiveTurnOwner = ""
	s.Busy = true
	s.BusyStarted = started
	s.Activity = activity
	s.Cancel = cancel
	s.Events = nil
}

// FinishTool marks tool execution complete and resets progress.
func (s *State) FinishTool() {
	if s == nil {
		return
	}
	s.FinishTurn()
	s.Progress = Progress{}
}
