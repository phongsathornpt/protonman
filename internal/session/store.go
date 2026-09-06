// Package session persists small, safety-relevant Proton session state.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/projectTHORN/proton/internal/agentprompt"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
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
	// PermissionMode is the configured mode spelling, not an enum number.
	PermissionMode string `json:"permission_mode"`
	// ActiveSkills records skills activated in this session.
	ActiveSkills []string `json:"active_skills,omitempty"`
	// AgentProfile records the active named coding profile without persisting a generated system prompt.
	AgentProfile string `json:"agent_profile,omitempty"`
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
	Role       model.Role `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToModelMessages converts persisted session messages to provider-neutral model messages.
// Legacy redacted tool protocol groups are compacted to plain assistant history
// so resume never fabricates empty tool arguments.
func ToModelMessages(stored []Message) []model.Message {
	stored = compactToolHistory(stored)
	messages := make([]model.Message, 0, len(stored))
	for _, message := range stored {
		if message.Role == model.RoleSystem && agentprompt.IsManaged(message.Content) {
			continue
		}
		messages = append(messages, model.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return messages
}

// FromModelMessages converts provider-neutral model messages to persisted session messages.
// Tool arguments are never copied into the stored representation. Tool protocol
// groups are compacted to plain text before they leave process memory.
func FromModelMessages(messages []model.Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == model.RoleSystem && agentprompt.IsManaged(message.Content) {
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

// ErrInvalidSessionID indicates that an ID could escape the session store
// directory or otherwise cannot name a state file safely.
var ErrInvalidSessionID = errors.New("invalid session id")

// FileStore stores one JSON state file per session ID.
type FileStore struct {
	root string
}

// NewFileStore creates a session store rooted at a private directory.
func NewFileStore(root string) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("session store root is required")
	}
	return &FileStore{root: root}, nil
}

// Load reads a session state. A missing state is returned as found=false.
func (s *FileStore) Load(ctx context.Context, sessionID string) (State, bool, error) {
	if err := validateSessionID(sessionID); err != nil {
		return State{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, false, fmt.Errorf("before loading session: %w", err)
	}

	file, err := os.Open(s.path(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("open session state: %w", err)
	}
	var state State
	decodeErr := json.NewDecoder(file).Decode(&state)
	closeErr := file.Close()
	if decodeErr != nil {
		return State{}, false, fmt.Errorf("decode session state: %w", decodeErr)
	}
	if closeErr != nil {
		return State{}, false, fmt.Errorf("close session state: %w", closeErr)
	}
	if state.Version != currentStateVersion {
		return State{}, false, fmt.Errorf("session state version %d is unsupported", state.Version)
	}
	if _, err := permission.ParseMode(state.PermissionMode); err != nil {
		return State{}, false, fmt.Errorf("session permission mode: %w", err)
	}
	if err := validateMessages(state.Messages); err != nil {
		return State{}, false, fmt.Errorf("session messages: %w", err)
	}
	state.Messages = sanitizeMessages(state.Messages)
	if err := validateMessages(state.Messages); err != nil {
		return State{}, false, fmt.Errorf("session messages: %w", err)
	}
	return state, true, nil
}

// LatestSession finds the most recently updated session matching prefix.
// If prefix is empty, all valid session files in the store are considered.
// It returns the session ID, state, found, and any error encountered.
func (s *FileStore) LatestSession(ctx context.Context, prefix string) (string, State, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", State{}, false, fmt.Errorf("before finding latest session: %w", err)
	}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return "", State{}, false, nil
	}
	if err != nil {
		return "", State{}, false, fmt.Errorf("read session directory: %w", err)
	}

	type candidate struct {
		id      string
		modTime time.Time
	}
	var candidates []candidate

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateSessionID(id); err != nil {
			continue
		}
		if prefix != "" {
			if id != prefix && !strings.HasPrefix(id, prefix+"-") {
				continue
			}
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{id: id, modTime: info.ModTime()})
	}

	if len(candidates) == 0 {
		return "", State{}, false, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].id > candidates[j].id
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})

	for _, cand := range candidates {
		state, found, err := s.Load(ctx, cand.id)
		if err != nil {
			continue
		}
		if found {
			return cand.id, state, true, nil
		}
	}

	return "", State{}, false, nil
}

// Save atomically writes the current session state with private file modes.
func (s *FileStore) Save(ctx context.Context, sessionID string, state State) (saveErr error) {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before saving session: %w", err)
	}
	if state.Version == 0 {
		state.Version = currentStateVersion
	}
	if state.Version != currentStateVersion {
		return fmt.Errorf("session state version %d is unsupported", state.Version)
	}
	if _, err := permission.ParseMode(state.PermissionMode); err != nil {
		return fmt.Errorf("session permission mode: %w", err)
	}
	if err := validateMessages(state.Messages); err != nil {
		return fmt.Errorf("session messages: %w", err)
	}
	state.Messages = sanitizeMessages(state.Messages)
	if err := validateMessages(state.Messages); err != nil {
		return fmt.Errorf("session messages: %w", err)
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}

	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	if err := os.Chmod(s.root, 0o700); err != nil {
		return fmt.Errorf("protect session store: %w", err)
	}
	file, err := os.CreateTemp(s.root, ".session-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary session state: %w", err)
	}
	temporaryPath := file.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil && saveErr == nil {
				saveErr = fmt.Errorf("close temporary session state: %w", closeErr)
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && saveErr == nil {
			saveErr = fmt.Errorf("remove temporary session state: %w", removeErr)
		}
	}()

	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect temporary session state: %w", err)
	}
	if err := json.NewEncoder(file).Encode(state); err != nil {
		return fmt.Errorf("encode session state: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync session state: %w", err)
	}
	closeErr := file.Close()
	closed = true
	if closeErr != nil {
		return fmt.Errorf("close session state: %w", closeErr)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before installing session state: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path(sessionID)); err != nil {
		return fmt.Errorf("install session state: %w", err)
	}
	return nil
}

// Delete removes a session state file.
func (s *FileStore) Delete(ctx context.Context, sessionID string) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before deleting session: %w", err)
	}
	err := os.Remove(s.path(sessionID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete session state: %w", err)
	}
	return nil
}

// List returns all valid session IDs matching the prefix, sorted newest first.
func (s *FileStore) List(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("before listing sessions: %w", err)
	}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session directory: %w", err)
	}

	type candidate struct {
		id      string
		modTime time.Time
	}
	var candidates []candidate

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateSessionID(id); err != nil {
			continue
		}
		if prefix != "" {
			if id != prefix && !strings.HasPrefix(id, prefix+"-") {
				continue
			}
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{id: id, modTime: info.ModTime()})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].id > candidates[j].id
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})

	ids := make([]string, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.id)
	}
	return ids, nil
}

func (s *FileStore) path(sessionID string) string {
	return filepath.Join(s.root, sessionID+".json")
}

func compactToolHistory(messages []Message) []Message {
	if len(messages) == 0 {
		return []Message{}
	}

	compacted := make([]Message, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		if message.Role == model.RoleAssistant && len(message.ToolCalls) > 0 {
			if text := strings.TrimSpace(message.Content); text != "" {
				compacted = append(compacted, Message{Role: model.RoleAssistant, Content: text})
			}

			results := make(map[string]Message, len(message.ToolCalls))
			next := index + 1
			for next < len(messages) && messages[next].Role == model.RoleTool {
				results[messages[next].ToolCallID] = messages[next]
				next++
			}
			for _, call := range message.ToolCalls {
				result, ok := results[call.ID]
				compacted = append(compacted, Message{
					Role:    model.RoleAssistant,
					Content: compactToolResult(call.Name, result, ok),
				})
				delete(results, call.ID)
			}
			for _, result := range results {
				compacted = append(compacted, Message{
					Role:    model.RoleAssistant,
					Content: compactToolResult(result.ToolName, result, true),
				})
			}
			index = next
			continue
		}
		if message.Role == model.RoleTool {
			compacted = append(compacted, Message{
				Role:    model.RoleAssistant,
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

func validateMessages(messages []Message) error {
	for _, message := range messages {
		if err := (model.Message{
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

func toModelToolCalls(calls []ToolCall) []model.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	converted := make([]model.ToolCall, 0, len(calls))
	for _, call := range calls {
		converted = append(converted, model.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: []byte(`{}`),
		})
	}
	return converted
}

func fromModelToolCalls(calls []model.ToolCall) []ToolCall {
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
