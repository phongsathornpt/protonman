package domain

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
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

func (p ContentPart) Validate() error {
	switch p.Type {
	case ContentPartText:
		if p.MIMEType != "" || p.Data != "" {
			return fmt.Errorf("%w: text content part cannot carry image metadata", ErrInvalidRequest)
		}
		return nil
	case ContentPartImage:
		mime := strings.ToLower(strings.TrimSpace(p.MIMEType))
		if !strings.HasPrefix(mime, "image/") || len(mime) <= len("image/") {
			return fmt.Errorf("%w: image content part requires an image MIME type", ErrInvalidRequest)
		}
		if strings.TrimSpace(p.Data) == "" {
			return fmt.Errorf("%w: image content part requires base64 data", ErrInvalidRequest)
		}
		decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(p.Data))
		if _, err := io.Copy(io.Discard, decoder); err != nil {
			return fmt.Errorf("%w: image content part has invalid base64 data: %v", ErrInvalidRequest, err)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported content part type %q", ErrInvalidRequest, p.Type)
	}
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
	ID      string
	Role    Role
	Content string
	// ReasoningContent preserves provider reasoning required by some models when
	// a tool call is followed by another request in thinking mode.
	ReasoningContent  string
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
	return formatFallbackMessageID(time.Now().UnixNano(), messageIDFallbackSeq.Add(1))
}

func formatFallbackMessageID(timestamp int64, seq uint64) string {
	return fmt.Sprintf("%sfallback_%016x_%08x", messageIDPrefix, timestamp, seq)
}

func ValidMessageID(id string) bool {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, messageIDPrefix) || len(id) <= len(messageIDPrefix) || len(id) > 128 {
		return false
	}
	for _, r := range id[len(messageIDPrefix):] {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func (m Message) Validate() error {
	if m.ID != "" && !ValidMessageID(m.ID) {
		return fmt.Errorf("%w: message id %q is invalid", ErrInvalidRequest, m.ID)
	}
	if !validRole(m.Role) {
		return fmt.Errorf("%w: unknown role %q", ErrInvalidRequest, m.Role)
	}
	if m.Role == RoleTool && strings.TrimSpace(m.ToolCallID) == "" {
		return fmt.Errorf("%w: tool message requires tool_call_id", ErrInvalidRequest)
	}
	for _, part := range m.Parts {
		if err := part.Validate(); err != nil {
			return err
		}
	}
	for _, call := range m.ToolCalls {
		if err := validateToolCall(call, ErrInvalidRequest); err != nil {
			return err
		}
	}
	return nil
}

func (m Message) TextContent() string {
	if m.Content != "" || len(m.Parts) == 0 {
		return m.Content
	}
	var builder strings.Builder
	for _, part := range m.Parts {
		if part.Type != ContentPartText || part.Text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(part.Text)
	}
	return builder.String()
}

func EnsureMessageIDs(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	hasMissing := false
	for _, message := range messages {
		if strings.TrimSpace(message.ID) == "" {
			hasMissing = true
			break
		}
	}
	if !hasMissing {
		return messages
	}
	cloned := make([]Message, 0, len(messages))
	for _, message := range messages {
		next := message
		if strings.TrimSpace(next.ID) == "" {
			next.ID = NewMessageID()
		}
		cloned = append(cloned, next)
	}
	return cloned
}

func CloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
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
