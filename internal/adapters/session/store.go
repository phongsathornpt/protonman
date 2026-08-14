// Package session persists small, safety-relevant Proton session state.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/domain/permission"
)

const currentStateVersion = 1

// State is the persisted portion of a Proton session.
type State struct {
	// Version allows incompatible state formats to fail closed.
	Version int `json:"version"`
	// PermissionMode is the configured mode spelling, not an enum number.
	PermissionMode string `json:"permission_mode"`
	// UpdatedAt records the last successful save.
	UpdatedAt time.Time `json:"updated_at"`
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
	return state, true, nil
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

func (s *FileStore) path(sessionID string) string {
	return filepath.Join(s.root, sessionID+".json")
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
