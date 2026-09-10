// Package protonsdk defines the provider-neutral model boundary used by Protonman agents.
package protonsdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ErrInvalidRequest   = errors.New("invalid model request")
	ErrInvalidEvent     = errors.New("invalid model event")
	ErrIncompleteStream = errors.New("incomplete model stream")
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ContentPartType string

const (
	ContentPartText  ContentPartType = "text"
	ContentPartImage ContentPartType = "image"
)

type ContentPart struct {
	Type     ContentPartType `json:"type"`
	Text     string          `json:"text,omitempty"`
	MIMEType string          `json:"mime_type,omitempty"`
	Data     string          `json:"data,omitempty"`
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

func (c ToolCall) Validate() error {
	return validateToolCall(c, ErrInvalidEvent)
}

func validateToolCall(c ToolCall, sentinel error) error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: tool call id is required", sentinel)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: tool name is required", sentinel)
	}
	if len(c.Arguments) > 0 && !json.Valid(c.Arguments) {
		return fmt.Errorf("%w: tool arguments must be valid JSON", sentinel)
	}
	return nil
}

type Tool struct {
	Name            string
	Description     string
	InputSchema     map[string]any
	OutputSchema    map[string]any
	ProviderOptions ProviderOptions
	Dynamic         bool
}

// ToolResult is the provider-neutral result returned to a model after an agent
// executes a tool call. Execution policy remains owned by the agent runtime.
type ToolResult struct {
	ToolCallID string
	ToolName   string
	Content    string
	Parts      []ContentPart
	IsError    bool
}

func (r ToolResult) Validate() error {
	if strings.TrimSpace(r.ToolCallID) == "" {
		return fmt.Errorf("%w: tool result call id is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(r.ToolName) == "" {
		return fmt.Errorf("%w: tool result name is required", ErrInvalidRequest)
	}
	return nil
}

func (t Tool) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("%w: tool name is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(t.Description) == "" {
		return fmt.Errorf("%w: description is required for %q", ErrInvalidRequest, t.Name)
	}
	return nil
}

type Message struct {
	// ID is a stable provider-neutral identity for this logical conversation message.
	// Providers may ignore it on the wire; runtimes should preserve it across cloning,
	// persistence, retention, and compaction. Legacy messages may omit it.
	ID                string
	Role              Role
	Content           string
	Parts             []ContentPart
	ToolCallID        string
	ToolName          string
	ToolResultIsError bool
	ToolCalls         []ToolCall
}

const messageIDPrefix = "msg_"

var messageIDFallbackSeq atomic.Uint64

// NewMessageID returns a stable opaque identity suitable for one logical message.
func NewMessageID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return messageIDPrefix + hex.EncodeToString(raw[:])
	}
	// crypto/rand failure is exceptionally rare. Preserve API infallibility with
	// a process-unique fallback rather than returning an empty identity.
	return fmt.Sprintf("%sf%016x%016x", messageIDPrefix, uint64(time.Now().UnixNano()), messageIDFallbackSeq.Add(1))
}

// EnsureMessageIDs clones messages and fills IDs only where they are missing.
// Existing IDs are never rewritten.
func EnsureMessageIDs(messages []Message) []Message {
	cloned := CloneMessages(messages)
	for i := range cloned {
		if strings.TrimSpace(cloned[i].ID) == "" {
			cloned[i].ID = NewMessageID()
		}
	}
	return cloned
}

func (m Message) TextContent() string {
	if m.Content != "" {
		return m.Content
	}
	var builder strings.Builder
	for _, part := range m.Parts {
		if part.Type == ContentPartText && part.Text != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

func ValidMessageID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return true // legacy messages are accepted and can be upgraded at runtime boundaries
	}
	if len(id) > 128 {
		return false
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func (m Message) Validate() error {
	if !ValidMessageID(m.ID) {
		return fmt.Errorf("%w: invalid message id %q", ErrInvalidRequest, m.ID)
	}
	if !validRole(m.Role) {
		return fmt.Errorf("%w: unsupported message role %q", ErrInvalidRequest, m.Role)
	}
	if m.Role != RoleAssistant && len(m.ToolCalls) > 0 {
		return fmt.Errorf("%w: only assistant messages can contain tool calls", ErrInvalidRequest)
	}
	if m.Role != RoleTool && m.ToolResultIsError {
		return fmt.Errorf("%w: only tool messages can be marked as tool errors", ErrInvalidRequest)
	}
	if m.Role == RoleTool && strings.TrimSpace(m.ToolCallID) == "" {
		return fmt.Errorf("%w: tool message call id is required", ErrInvalidRequest)
	}
	for _, call := range m.ToolCalls {
		if err := validateToolCall(call, ErrInvalidRequest); err != nil {
			return err
		}
	}
	return nil
}

type ProviderOptions map[string]json.RawMessage
type ProviderMetadata map[string]json.RawMessage

type ToolChoice string

const (
	ToolChoiceAuto     ToolChoice = ""
	ToolChoiceRequired ToolChoice = "required"
)

// ReasoningEffort is the provider-neutral reasoning intensity requested from a model.
// The empty value preserves the model/provider default.
type ReasoningEffort string

const (
	ReasoningDefault ReasoningEffort = ""
	ReasoningNone    ReasoningEffort = "none"
	ReasoningMinimal ReasoningEffort = "minimal"
	ReasoningLow     ReasoningEffort = "low"
	ReasoningMedium  ReasoningEffort = "medium"
	ReasoningHigh    ReasoningEffort = "high"
	ReasoningXHigh   ReasoningEffort = "xhigh"
	ReasoningMax     ReasoningEffort = "max"
)

func ParseReasoningEffort(value string) (ReasoningEffort, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "auto" || value == "default" {
		return ReasoningDefault, nil
	}
	effort := ReasoningEffort(value)
	if !effort.Valid() {
		return ReasoningDefault, fmt.Errorf("%w: unsupported reasoning effort %q", ErrInvalidRequest, value)
	}
	return effort, nil
}

func (e ReasoningEffort) Valid() bool {
	switch e {
	case ReasoningDefault, ReasoningNone, ReasoningMinimal, ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningXHigh, ReasoningMax:
		return true
	default:
		return false
	}
}

type ModelOptions struct {
	MaxOutputTokens  int
	ToolChoice       ToolChoice
	ReasoningEffort  ReasoningEffort
	ProviderOptions  ProviderOptions
	IncludeRawChunks bool
}

type Request struct {
	Messages []Message
	Tools    []Tool
	Options  ModelOptions
}

func (r Request) Validate() error {
	if r.Options.MaxOutputTokens < 0 {
		return fmt.Errorf("%w: max output tokens cannot be negative", ErrInvalidRequest)
	}
	if r.Options.ToolChoice != ToolChoiceAuto && r.Options.ToolChoice != ToolChoiceRequired {
		return fmt.Errorf("%w: unsupported tool choice %q", ErrInvalidRequest, r.Options.ToolChoice)
	}
	if !r.Options.ReasoningEffort.Valid() {
		return fmt.Errorf("%w: unsupported reasoning effort %q", ErrInvalidRequest, r.Options.ReasoningEffort)
	}
	if r.Options.ToolChoice == ToolChoiceRequired && len(r.Tools) == 0 {
		return fmt.Errorf("%w: required tool choice needs at least one tool", ErrInvalidRequest)
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("%w: at least one message is required", ErrInvalidRequest)
	}
	for _, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
	}
	for _, tool := range r.Tools {
		if err := tool.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func CloneMessages(messages []Message) []Message {
	cloned := make([]Message, 0, len(messages))
	for _, message := range messages {
		clone := message
		clone.Parts = append([]ContentPart(nil), message.Parts...)
		clone.ToolCalls = make([]ToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			call.Arguments = append(json.RawMessage(nil), call.Arguments...)
			clone.ToolCalls = append(clone.ToolCalls, call)
		}
		cloned = append(cloned, clone)
	}
	return cloned
}

type FinishReason string

const (
	FinishStop      FinishReason = "stop"
	FinishLength    FinishReason = "length"
	FinishToolCalls FinishReason = "tool_calls"
	FinishError     FinishReason = "error"
	FinishOther     FinishReason = "other"
)

type Usage struct {
	InputTokens       int64
	OutputTokens      int64
	TotalTokens       int64
	CachedInputTokens int64
}

func (u Usage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 || u.CachedInputTokens < 0 {
		return fmt.Errorf("%w: token usage cannot be negative", ErrInvalidEvent)
	}
	return nil
}

type EventKind string

const (
	EventTextStart     EventKind = "text_start"
	EventTextDelta     EventKind = "text_delta"
	EventTextEnd       EventKind = "text_end"
	EventToolCallStart EventKind = "tool_call_start"
	EventToolCallDelta EventKind = "tool_call_delta"
	EventToolCallEnd   EventKind = "tool_call_end"
	// EventToolCall carries a complete tool call for agent runtimes that do not
	// need incremental argument rendering. Providers may emit both lifecycle
	// events and this normalized complete event.
	EventToolCall EventKind = "tool_call"
	EventUsage    EventKind = "usage"
	EventRaw      EventKind = "raw"
	EventFinish   EventKind = "finish"
)

type Event struct {
	Kind EventKind
	Text string

	ToolCall         ToolCall
	ToolCallID       string
	ToolName         string
	ArgumentsDelta   string
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata ProviderMetadata
	RawData          []byte
}

func (e Event) Validate() error {
	switch e.Kind {
	case EventTextStart, EventTextDelta, EventTextEnd:
		return nil
	case EventRaw:
		if len(e.RawData) == 0 {
			return fmt.Errorf("%w: raw event data is required", ErrInvalidEvent)
		}
		return nil
	case EventUsage:
		return e.Usage.Validate()
	case EventFinish:
		if !validFinishReason(e.FinishReason) {
			return fmt.Errorf("%w: unsupported finish reason %q", ErrInvalidEvent, e.FinishReason)
		}
		return nil
	case EventToolCallStart:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return fmt.Errorf("%w: tool call start id is required", ErrInvalidEvent)
		}
		if strings.TrimSpace(e.ToolName) == "" {
			return fmt.Errorf("%w: tool call start name is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCallDelta, EventToolCallEnd:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return fmt.Errorf("%w: tool call id is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCall:
		return e.ToolCall.Validate()
	default:
		return fmt.Errorf("%w: unsupported event kind %q", ErrInvalidEvent, e.Kind)
	}
}

type Stream interface {
	Next(ctx context.Context) (Event, error)
	Close() error
}

func validFinishReason(reason FinishReason) bool {
	switch reason {
	case FinishStop, FinishLength, FinishToolCalls, FinishError, FinishOther:
		return true
	default:
		return false
	}
}

func validRole(role Role) bool {
	switch role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}
