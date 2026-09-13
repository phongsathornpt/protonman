package protonsdk

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
			call.Arguments = append(json.RawMessage(nil), call.Arguments...)
			clone.ToolCalls = append(clone.ToolCalls, call)
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
