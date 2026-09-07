// Package app exposes application use-case boundaries to inbound adapters.
package app

import (
	"context"
	"fmt"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
	sdk "github.com/projectTHORN/proton/proton-sdk"
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

// ReasoningPolicy returns the explicit reasoning policy when the underlying
// conversation supports session-local reasoning control.
func ReasoningPolicy(conversation Conversation) (sdk.ReasoningEffort, bool) {
	loop, ok := conversation.(*turn.Loop)
	if !ok || loop == nil {
		return sdk.ReasoningDefault, false
	}
	return loop.ReasoningPolicy()
}

// CloneConversationWithReasoning returns an independent conversation with a
// session-local reasoning policy.
func CloneConversationWithReasoning(conversation Conversation, effort sdk.ReasoningEffort, explicit bool) (Conversation, error) {
	loop, ok := conversation.(*turn.Loop)
	if !ok || loop == nil {
		return nil, fmt.Errorf("conversation does not support reasoning overrides")
	}
	return loop.CloneWithReasoningEffort(effort, explicit)
}

// CloneConversationWithTools returns an independent conversation bound to a
// different tool-call service.
func CloneConversationWithTools(conversation Conversation, tools *toolcall.Service) (Conversation, error) {
	loop, ok := conversation.(*turn.Loop)
	if !ok || loop == nil {
		return nil, fmt.Errorf("conversation does not support tool rebinding")
	}
	return loop.CloneWithTools(tools)
}
