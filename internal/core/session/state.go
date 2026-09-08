// Package session persists small, safety-relevant Proton session state.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const (
	currentStateVersion = 1
	maxStoredMessages   = 200
	maxStoredContent    = 32 * 1024
)

// State is the persisted portion of a Proton session.
type State struct {
	// Version allows incompatible state formats to fail closed.
	Version int `json:"version"`
	// SessionID is the stable identity of this session. Legacy files may omit it.
	SessionID string `json:"session_id,omitempty"`
	// Revision is the durable optimistic-concurrency generation for this session state.
	Revision uint64 `json:"revision,omitempty"`
	// WorkspaceKey binds the session to the workspace it was created for without persisting an absolute path.
	WorkspaceKey string `json:"workspace_key,omitempty"`
	// WorkspaceName is a display-only basename for session discovery.
	WorkspaceName string `json:"workspace_name,omitempty"`
	// CreatedAt records the first successful save of the session.
	CreatedAt time.Time `json:"created_at,omitempty"`
	// PermissionMode is the configured mode spelling, not an enum number.
	PermissionMode string `json:"permission_mode"`
	// ActiveSkills records skills activated in this session.
	ActiveSkills []string `json:"active_skills,omitempty"`
	// AgentProfile records the active named coding profile without persisting a generated system prompt.
	AgentProfile string `json:"agent_profile,omitempty"`
	// ReasoningEffort records the session reasoning override ("auto" preserves provider/profile defaults).
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// Messages is the redacted conversation transcript. Tool arguments are
	// never stored.
	Messages []Message `json:"messages,omitempty"`
	// UpdatedAt records the last successful save.
	UpdatedAt time.Time `json:"updated_at"`
}

// ToolCall is the redacted identity of an assistant-requested tool call.
// Arguments are intentionally omitted from persisted session state.
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Message is one persisted conversation turn without tool arguments.
type Message struct {
	Role       sdk.Role   `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToModelMessages converts persisted session messages to provider-neutral model messages.
// Legacy redacted tool protocol groups are compacted to plain assistant history
// so resume never fabricates empty tool arguments.
func ToModelMessages(stored []Message) []sdk.Message {
	stored = compactToolHistory(stored)
	messages := make([]sdk.Message, 0, len(stored))
	for _, message := range stored {
		if message.Role == sdk.RoleSystem && isManagedSystemPrompt(message.Content) {
			continue
		}
		messages = append(messages, sdk.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return messages
}

// FromModelMessages converts provider-neutral model messages to persisted session messages.
// Tool arguments are never copied into the stored representation. Tool protocol
// groups are compacted to plain text before they leave process memory.
func FromModelMessages(messages []sdk.Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == sdk.RoleSystem && isManagedSystemPrompt(message.Content) {
			continue
		}
		out = append(out, Message{
			Role:       message.Role,
			Content:    message.Content,
			ToolName:   message.ToolName,
			ToolCallID: message.ToolCallID,
			ToolCalls:  fromModelToolCalls(message.ToolCalls),
		})
	}
	return compactToolHistory(out)
}

func isManagedSystemPrompt(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "<proton-system-prompt ") ||
		strings.HasPrefix(trimmed, "You are Proton, an autonomous coding agent operating inside a real workspace.") ||
		strings.HasPrefix(trimmed, "You are an Explorer subagent in Proton.") ||
		strings.HasPrefix(trimmed, "You are a Code Reviewer subagent in Proton.") ||
		strings.HasPrefix(trimmed, "You are a Worker subagent in Proton.") ||
		strings.HasPrefix(trimmed, "You are Proton in POW Mode") ||
		strings.HasPrefix(trimmed, "You are Proton in DEX Mode") ||
		strings.HasPrefix(trimmed, "You are Proton in INT Mode")
}

// Summary is a bounded, display-oriented view of one persisted session.
type Summary struct {
	ID              string    `json:"id"`
	WorkspaceKey    string    `json:"workspace_key,omitempty"`
	WorkspaceName   string    `json:"workspace_name,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
	AgentProfile    string    `json:"agent_profile,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	MessageCount    int       `json:"message_count"`
	Preview         string    `json:"preview,omitempty"`
}

// ListOptions bounds session discovery and optionally filters by workspace identity.
type ListOptions struct {
	WorkspaceKey string
	Prefix       string
	Limit        int
	Offset       int
}

// ErrInvalidSessionID indicates that an ID could escape the session store
// directory or otherwise cannot name a state file safely.
var ErrInvalidSessionID = errors.New("invalid session id")

// ErrRevisionConflict indicates that a stale session snapshot attempted to overwrite newer state.
var ErrRevisionConflict = errors.New("session revision conflict")

func legacyWorkspaceKey(sessionID string) string {
	const prefix = "workspace-"
	if !strings.HasPrefix(sessionID, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(sessionID, prefix)
	if idx := strings.IndexByte(rest, '-'); idx > 0 {
		return rest[:idx]
	}
	return ""
}

func compactToolHistory(messages []Message) []Message {
	if len(messages) == 0 {
		return []Message{}
	}

	compacted := make([]Message, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		if message.Role == sdk.RoleAssistant && len(message.ToolCalls) > 0 {
			if text := strings.TrimSpace(message.Content); text != "" {
				compacted = append(compacted, Message{Role: sdk.RoleAssistant, Content: text})
			}

			results := make(map[string]Message, len(message.ToolCalls))
			next := index + 1
			for next < len(messages) && messages[next].Role == sdk.RoleTool {
				results[messages[next].ToolCallID] = messages[next]
				next++
			}
			for _, call := range message.ToolCalls {
				result, ok := results[call.ID]
				compacted = append(compacted, Message{
					Role:    sdk.RoleAssistant,
					Content: compactToolResult(call.Name, result, ok),
				})
				delete(results, call.ID)
			}
			for _, result := range results {
				compacted = append(compacted, Message{
					Role:    sdk.RoleAssistant,
					Content: compactToolResult(result.ToolName, result, true),
				})
			}
			index = next
			continue
		}
		if message.Role == sdk.RoleTool {
			compacted = append(compacted, Message{
				Role:    sdk.RoleAssistant,
				Content: compactToolResult(message.ToolName, message, true),
			})
			index++
			continue
		}
		message.ToolCalls = nil
		message.ToolName = ""
		message.ToolCallID = ""
		compacted = append(compacted, message)
		index++
	}
	return compacted
}

func compactToolResult(toolName string, message Message, found bool) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "unknown"
	}
	if !found {
		return fmt.Sprintf("Historical tool %s was requested, but its result was not persisted.", toolName)
	}

	var result tool.Result
	if err := json.Unmarshal([]byte(message.Content), &result); err == nil && (result.ToolName != "" || result.CallID != "" || result.Failure != nil) {
		if result.ToolName != "" {
			toolName = result.ToolName
		}
		if result.Failure != nil {
			return fmt.Sprintf("Historical tool %s failed [%s]: %s", toolName, result.Failure.Code, result.Failure.Message)
		}
		output := strings.TrimSpace(result.Output)
		if output == "" {
			return fmt.Sprintf("Historical tool %s completed with no text output.", toolName)
		}
		if result.Truncated && result.NextOffset != nil {
			return fmt.Sprintf("Historical tool %s result (truncated, next_offset=%d):\n%s", toolName, *result.NextOffset, output)
		}
		if result.Truncated {
			return fmt.Sprintf("Historical tool %s result (truncated):\n%s", toolName, output)
		}
		return fmt.Sprintf("Historical tool %s result:\n%s", toolName, output)
	}

	content := strings.TrimSpace(message.Content)
	if content == "" {
		return fmt.Sprintf("Historical tool %s completed with no text output.", toolName)
	}
	return fmt.Sprintf("Historical tool %s result:\n%s", toolName, content)
}

func sanitizeMessages(messages []Message) []Message {
	messages = compactToolHistory(messages)
	if len(messages) == 0 {
		return []Message{}
	}
	if len(messages) > maxStoredMessages {
		messages = messages[len(messages)-maxStoredMessages:]
	}
	cleaned := make([]Message, 0, len(messages))
	for _, message := range messages {
		message.Content = truncateStoredContent(message.Content)
		cleaned = append(cleaned, message)
	}
	return cleaned
}

func truncateStoredContent(content string) string {
	if len(content) <= maxStoredContent {
		return content
	}
	content = content[:maxStoredContent]
	for len(content) > 0 && !utf8.ValidString(content) {
		content = content[:len(content)-1]
	}
	return content
}

func validateReasoningSetting(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	_, err := sdk.ParseReasoningEffort(value)
	return err
}

func validateMessages(messages []Message) error {
	for _, message := range messages {
		if err := (sdk.Message{
			Role:       message.Role,
			Content:    message.Content,
			ToolName:   message.ToolName,
			ToolCallID: message.ToolCallID,
			ToolCalls:  toModelToolCalls(message.ToolCalls),
		}).Validate(); err != nil {
			return err
		}
	}
	return nil
}

func toModelToolCalls(calls []ToolCall) []sdk.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	converted := make([]sdk.ToolCall, 0, len(calls))
	for _, call := range calls {
		converted = append(converted, sdk.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: []byte(`{}`),
		})
	}
	return converted
}

func fromModelToolCalls(calls []sdk.ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	redacted := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		redacted = append(redacted, ToolCall{ID: call.ID, Name: call.Name})
	}
	return redacted
}

func validateSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" || sessionID == "." || sessionID == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	if filepath.Base(sessionID) != sessionID {
		return fmt.Errorf("%w: path separators are not allowed", ErrInvalidSessionID)
	}
	for _, character := range sessionID {
		allowed := character == '-' || character == '_' || character == '.' ||
			character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9'
		if !allowed {
			return fmt.Errorf("%w: unsupported character %q", ErrInvalidSessionID, character)
		}
	}
	return nil
}
