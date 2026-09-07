package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
)

// ValidateID rejects session identifiers that could escape a persistence root.
func ValidateID(sessionID string) error { return validateSessionID(sessionID) }

// NormalizeLoadedState validates and normalizes a state decoded by a repository adapter.
func NormalizeLoadedState(sessionID string, state State) (State, error) {
	if err := validateSessionID(sessionID); err != nil {
		return State{}, err
	}
	if state.Version != currentStateVersion {
		return State{}, fmt.Errorf("session state version %d is unsupported", state.Version)
	}
	if _, err := permission.ParseMode(state.PermissionMode); err != nil {
		return State{}, fmt.Errorf("session permission mode: %w", err)
	}
	if err := validateReasoningSetting(state.ReasoningEffort); err != nil {
		return State{}, fmt.Errorf("session reasoning effort: %w", err)
	}
	if err := validateMessages(state.Messages); err != nil {
		return State{}, fmt.Errorf("session messages: %w", err)
	}
	state.Messages = sanitizeMessages(state.Messages)
	if err := validateMessages(state.Messages); err != nil {
		return State{}, fmt.Errorf("session messages: %w", err)
	}
	if state.SessionID == "" {
		state.SessionID = sessionID
	}
	if state.WorkspaceKey == "" {
		state.WorkspaceKey = legacyWorkspaceKey(sessionID)
	}
	if state.CreatedAt.IsZero() && !state.UpdatedAt.IsZero() {
		state.CreatedAt = state.UpdatedAt
	}
	return state, nil
}

// PrepareStateForSave validates, sanitizes, and timestamps state before a
// repository adapter serializes it.
func PrepareStateForSave(sessionID string, state State, existing *State, now time.Time) (State, error) {
	if err := validateSessionID(sessionID); err != nil {
		return State{}, err
	}
	if state.Version == 0 {
		state.Version = currentStateVersion
	}
	state.SessionID = sessionID
	if state.WorkspaceKey == "" {
		state.WorkspaceKey = legacyWorkspaceKey(sessionID)
	}
	if existing != nil {
		if state.CreatedAt.IsZero() {
			state.CreatedAt = existing.CreatedAt
		}
		if state.WorkspaceName == "" {
			state.WorkspaceName = existing.WorkspaceName
		}
	}
	if state.Version != currentStateVersion {
		return State{}, fmt.Errorf("session state version %d is unsupported", state.Version)
	}
	if _, err := permission.ParseMode(state.PermissionMode); err != nil {
		return State{}, fmt.Errorf("session permission mode: %w", err)
	}
	if err := validateReasoningSetting(state.ReasoningEffort); err != nil {
		return State{}, fmt.Errorf("session reasoning effort: %w", err)
	}
	if err := validateMessages(state.Messages); err != nil {
		return State{}, fmt.Errorf("session messages: %w", err)
	}
	state.Messages = sanitizeMessages(state.Messages)
	if err := validateMessages(state.Messages); err != nil {
		return State{}, fmt.Errorf("session messages: %w", err)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	state.UpdatedAt = now
	return state, nil
}

// Preview returns the first bounded user-message summary for session discovery.
func Preview(messages []Message) string {
	for _, message := range messages {
		if message.Role != model.RoleUser {
			continue
		}
		text := strings.Join(strings.Fields(message.Content), " ")
		if text == "" {
			continue
		}
		const maxRunes = 100
		runes := []rune(text)
		if len(runes) > maxRunes {
			return string(runes[:maxRunes-1]) + "…"
		}
		return text
	}
	return ""
}
