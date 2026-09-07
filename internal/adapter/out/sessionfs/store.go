// Package sessionfs implements the filesystem session repository adapter.
package sessionfs

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

	"github.com/projectTHORN/proton/internal/core/session"
)

type State = session.State
type Summary = session.Summary
type ListOptions = session.ListOptions

var _ session.Repository = (*FileStore)(nil)

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
	if err := session.ValidateID(sessionID); err != nil {
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
	state, err = session.NormalizeLoadedState(sessionID, state)
	if err != nil {
		return State{}, false, err
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
		if err := session.ValidateID(id); err != nil {
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
	if err := session.ValidateID(sessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before saving session: %w", err)
	}
	var existing *State
	if loaded, found, loadErr := s.Load(ctx, sessionID); loadErr == nil && found {
		existing = &loaded
	}
	state, err := session.PrepareStateForSave(sessionID, state, existing, time.Now().UTC())
	if err != nil {
		return err
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
	if err := session.ValidateID(sessionID); err != nil {
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
		if err := session.ValidateID(id); err != nil {
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

// ListSummaries returns bounded session metadata sorted by most recently updated.
func (s *FileStore) ListSummaries(ctx context.Context, options ListOptions) ([]Summary, error) {
	ids, err := s.List(ctx, options.Prefix)
	if err != nil {
		return nil, err
	}
	summaries := make([]Summary, 0, len(ids))
	for _, id := range ids {
		state, found, loadErr := s.Load(ctx, id)
		if loadErr != nil || !found {
			continue
		}
		if options.WorkspaceKey != "" && state.WorkspaceKey != options.WorkspaceKey {
			continue
		}
		summaries = append(summaries, Summary{
			ID: id, WorkspaceKey: state.WorkspaceKey, WorkspaceName: state.WorkspaceName,
			CreatedAt: state.CreatedAt, UpdatedAt: state.UpdatedAt, AgentProfile: state.AgentProfile,
			ReasoningEffort: state.ReasoningEffort, MessageCount: len(state.Messages), Preview: session.Preview(state.Messages),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt.Equal(summaries[j].UpdatedAt) {
			return summaries[i].ID > summaries[j].ID
		}
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(summaries) {
		return []Summary{}, nil
	}
	summaries = summaries[offset:]
	if options.Limit > 0 && len(summaries) > options.Limit {
		summaries = summaries[:options.Limit]
	}
	return summaries, nil
}

func (s *FileStore) path(sessionID string) string {
	return filepath.Join(s.root, sessionID+".json")
}
