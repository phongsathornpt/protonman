// Package app exposes application use-case boundaries to inbound adapters.
package app

import (
	"context"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/turn"
)

// Conversation executes one model/tool turn over an existing message history.
// Inbound adapters depend on this port instead of the concrete turn loop.
type Conversation interface {
	Run(context.Context, []model.Message, turn.Sink) (turn.Result, error)
}

// Event and Result are application-level aliases used by inbound adapters.
type Event = turn.Event
type Result = turn.Result
type Sink = turn.Sink

const (
	EventTextDelta  = turn.EventTextDelta
	EventToolCall   = turn.EventToolCall
	EventToolResult = turn.EventToolResult
	EventCompleted  = turn.EventCompleted
	EventFailed     = turn.EventFailed
)

var (
	ErrToolDispatchUnavailable = turn.ErrToolDispatchUnavailable
	ErrUnresolvedToolCall      = turn.ErrUnresolvedToolCall
)
