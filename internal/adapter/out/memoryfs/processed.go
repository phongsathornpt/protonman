package memoryfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type processedFile struct {
	Version   int               `json:"version"`
	Revisions map[string]uint64 `json:"revisions"`
}

func (s *FileStore) ProcessedRevision(ctx context.Context, sessionID string) (uint64, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if !safeSegment(sessionID) {
		return 0, false, fmt.Errorf("invalid memory session id %q", sessionID)
	}
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	state, err := s.loadProcessed()
	if err != nil {
		return 0, false, err
	}
	revision, ok := state.Revisions[sessionID]
	return revision, ok, nil
}

func (s *FileStore) MarkProcessed(ctx context.Context, sessionID string, revision uint64) error {
	sessionID = strings.TrimSpace(sessionID)
	if !safeSegment(sessionID) {
		return fmt.Errorf("invalid memory session id %q", sessionID)
	}
	dir := filepath.Join(s.root, "v1", "sources")
	return s.withScopeLock(ctx, dir, func() error {
		state, err := s.loadProcessed()
		if err != nil {
			return err
		}
		if previous, ok := state.Revisions[sessionID]; ok && previous >= revision {
			return nil
		}
		state.Revisions[sessionID] = revision
		return s.writeProcessed(ctx, state)
	})
}

func (s *FileStore) loadProcessed() (processedFile, error) {
	path := filepath.Join(s.root, "v1", "sources", "processed.json")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return processedFile{Version: schemaVersion, Revisions: map[string]uint64{}}, nil
	}
	if err != nil {
		return processedFile{}, fmt.Errorf("open processed memory revisions: %w", err)
	}
	defer file.Close()
	var state processedFile
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return processedFile{}, fmt.Errorf("decode processed memory revisions: %w", err)
	}
	if state.Version != schemaVersion {
		return processedFile{}, fmt.Errorf("unsupported processed memory version %d", state.Version)
	}
	if state.Revisions == nil {
		state.Revisions = map[string]uint64{}
	}
	return state, nil
}

func (s *FileStore) writeProcessed(ctx context.Context, state processedFile) (writeErr error) {
	dir := filepath.Join(s.root, "v1", "sources")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create processed memory directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".processed-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary processed memory file: %w", err)
	}
	temporaryPath := file.Name()
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encode processed memory revisions: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync processed memory revisions: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close processed memory revisions: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filepath.Join(dir, "processed.json")); err != nil {
		return fmt.Errorf("install processed memory revisions: %w", err)
	}
	return nil
}
