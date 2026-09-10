package runtime

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

func (s *turnModelState) beginTurn(owner string, started time.Time) {
	s.turnProgress = turnProgress{}
	s.activeTurnOwner = owner
	s.busy = true
	s.busyStarted = started
	s.activity = "analyzing"
}

func (s *turnModelState) bindTurn(cancel context.CancelFunc, events <-chan tea.Msg) {
	s.turnCancel = cancel
	s.turnEvents = events
}

func (s *turnModelState) finishTurn() {
	s.busy = false
	s.busyStarted = time.Time{}
	s.activity = "ready"
	s.turnCancel = nil
	s.turnEvents = nil
	s.activeTurnOwner = ""
}
func (s *turnModelState) beginTool(activity string, started time.Time, cancel context.CancelFunc) {
	s.turnProgress = turnProgress{}
	s.activeTurnOwner = ""
	s.busy = true
	s.busyStarted = started
	s.activity = activity
	s.turnCancel = cancel
	s.turnEvents = nil
}

func (s *turnModelState) finishTool() {
	s.finishTurn()
	s.turnProgress = turnProgress{}
}
