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

	"github.com/phongsathornpt/proton/internal/core/session"
)

type State = session.State
type Summary = session.Summary
type ListOptions = session.ListOptions

var _ session.Repository = (*FileStore)(nil)

// FileStore stores each session as an aggregate directory below root.
// New layout: <root>/<session-id>/state.json. Legacy <root>/<session-id>.json
// files remain readable and are removed after the next successful save.
type FileStore struct{ root string }

func NewFileStore(root string) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("session store root is required")
	}
	return &FileStore{root: root}, nil
}

func (s *FileStore) Load(ctx context.Context, sessionID string) (State, bool, error) {
	if err := session.ValidateID(sessionID); err != nil {
		return State{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, false, fmt.Errorf("before loading session: %w", err)
	}
	path := s.path(sessionID)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		path = s.legacyPath(sessionID)
		file, err = os.Open(path)
	}
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

func (s *FileStore) LatestSession(ctx context.Context, prefix string) (string, State, bool, error) {
	ids, err := s.List(ctx, prefix)
	if err != nil {
		return "", State{}, false, err
	}
	for _, id := range ids {
		state, found, loadErr := s.Load(ctx, id)
		if loadErr == nil && found {
			return id, state, true, nil
		}
	}
	return "", State{}, false, nil
}

func (s *FileStore) Save(ctx context.Context, sessionID string, state State) error {
	if err := session.ValidateID(sessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before saving session: %w", err)
	}
	return s.withSessionLock(ctx, sessionID, func() error { return s.saveLocked(ctx, sessionID, state) })
}

func (s *FileStore) saveLocked(ctx context.Context, sessionID string, state State) (saveErr error) {
	var existing *State
	if loaded, found, loadErr := s.Load(ctx, sessionID); loadErr != nil {
		return loadErr
	} else if found {
		existing = &loaded
	}
	prepared, err := session.PrepareStateForSave(sessionID, state, existing, time.Now().UTC())
	if err != nil {
		return err
	}
	resources, err := session.ResolveResources(s.root, sessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(resources.Root, 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}
	if err := os.Chmod(resources.Root, 0o700); err != nil {
		return fmt.Errorf("protect session directory: %w", err)
	}
	file, err := os.CreateTemp(resources.Root, ".state-*.tmp")
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
	if err := json.NewEncoder(file).Encode(prepared); err != nil {
		return fmt.Errorf("encode session state: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync session state: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close session state: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before installing session state: %w", err)
	}
	if err := os.Rename(temporaryPath, resources.State); err != nil {
		return fmt.Errorf("install session state: %w", err)
	}
	_ = os.Remove(s.legacyPath(sessionID))
	return nil
}

func (s *FileStore) Delete(ctx context.Context, sessionID string) error {
	if err := session.ValidateID(sessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before deleting session: %w", err)
	}
	return s.withSessionLock(ctx, sessionID, func() error {
		resources, err := session.ResolveResources(s.root, sessionID)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(resources.Root); err != nil {
			return fmt.Errorf("delete session directory: %w", err)
		}
		if err := os.Remove(s.legacyPath(sessionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete legacy session state: %w", err)
		}
		return nil
	})
}

const sessionLockStaleAfter = 2 * time.Minute

func (s *FileStore) withSessionLock(ctx context.Context, sessionID string, fn func() error) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	if err := os.Chmod(s.root, 0o700); err != nil {
		return fmt.Errorf("protect session store: %w", err)
	}
	lockPath := filepath.Join(s.root, "."+sessionID+".lock")
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := os.Mkdir(lockPath, 0o700)
		if err == nil {
			defer os.Remove(lockPath)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("acquire session lock: %w", err)
		}
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > sessionLockStaleAfter {
			_ = os.Remove(lockPath)
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type candidate struct {
	id      string
	modTime time.Time
}

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
	byID := make(map[string]candidate, len(entries))
	for _, entry := range entries {
		id := ""
		statePath := ""
		if entry.IsDir() {
			id = entry.Name()
			statePath = filepath.Join(s.root, id, session.StateFileName)
			if _, statErr := os.Stat(statePath); statErr != nil {
				continue
			}
		} else if strings.HasSuffix(entry.Name(), ".json") {
			id = strings.TrimSuffix(entry.Name(), ".json")
			statePath = filepath.Join(s.root, entry.Name())
		} else {
			continue
		}
		if err := session.ValidateID(id); err != nil {
			continue
		}
		if prefix != "" && id != prefix && !strings.HasPrefix(id, prefix+"-") {
			continue
		}
		info, statErr := os.Stat(statePath)
		if statErr != nil {
			continue
		}
		cand := candidate{id: id, modTime: info.ModTime()}
		if previous, exists := byID[id]; !exists || cand.modTime.After(previous.modTime) {
			byID[id] = cand
		}
	}
	candidates := make([]candidate, 0, len(byID))
	for _, cand := range byID {
		candidates = append(candidates, cand)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].id > candidates[j].id
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	ids := make([]string, 0, len(candidates))
	for _, cand := range candidates {
		ids = append(ids, cand.id)
	}
	return ids, nil
}

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
	resources, _ := session.ResolveResources(s.root, sessionID)
	return resources.State
}

func (s *FileStore) legacyPath(sessionID string) string {
	return filepath.Join(s.root, sessionID+".json")
}
