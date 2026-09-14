package protonsdk

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
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
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		isPunct := r == '_' || r == '-' || r == '.' || r == ':'
		if isLower || isUpper || isDigit || isPunct {
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

func CloneMessages(messages []Message) []Message {
	cloned := make([]Message, 0, len(messages))
	for _, message := range messages {
		clone := message
		clone.Parts = append([]ContentPart(nil), message.Parts...)
		clone.ToolCalls = make([]ToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			clone.ToolCalls = append(clone.ToolCalls, call.Clone())
		}
		cloned = append(cloned, clone)
	}
	return cloned
}

func validRole(role Role) bool {
	switch role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}

// AppendAssistantResponse appends a normalized assistant response to conversation
// history. Text and tool calls stay on the same logical assistant message;
// provider adapters may split them on the wire if required.
func AppendAssistantResponse(messages []Message, response Response) []Message {
	next := CloneMessages(messages)
	if response.Text == "" && len(response.ToolCalls) == 0 {
		return next
	}
	assistant := Message{
		ID:        NewMessageID(),
		Role:      RoleAssistant,
		Content:   response.Text,
		ToolCalls: make([]ToolCall, 0, len(response.ToolCalls)),
	}
	for _, call := range response.ToolCalls {
		assistant.ToolCalls = append(assistant.ToolCalls, call.Clone())
	}
	return append(next, assistant)
}

// AppendAssistantStep is retained for source compatibility with earlier SDK releases.
// Deprecated: use AppendAssistantResponse.
func AppendAssistantStep(messages []Message, result StepResult) []Message {
	return AppendAssistantResponse(messages, result)
}

// AppendToolResults appends tool execution outputs in model-history order.
func AppendToolResults(messages []Message, results []ToolResult) ([]Message, error) {
	next := CloneMessages(messages)
	for _, result := range results {
		if err := result.Validate(); err != nil {
			return nil, err
		}
		next = append(next, Message{
			ID:                NewMessageID(),
			Role:              RoleTool,
			Content:           result.Content,
			Parts:             append([]ContentPart(nil), result.Parts...),
			ToolCallID:        result.ToolCallID,
			ToolName:          result.ToolName,
			ToolResultIsError: result.IsError,
		})
	}
	return next, nil
}
