package memoryfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

const lockStaleAfter = 2 * time.Minute

func (s *FileStore) loadUnlocked(dir string, scope memory.Scope, workspaceKey string) ([]memory.Entry, error) {
	file, err := os.Open(filepath.Join(dir, "index.json"))
	if errors.Is(err, os.ErrNotExist) {
		return []memory.Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open memory index: %w", err)
	}
	var index indexFile
	decodeErr := json.NewDecoder(file).Decode(&index)
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, fmt.Errorf("decode memory index: %w", decodeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close memory index: %w", closeErr)
	}
	if index.Version != schemaVersion {
		return nil, fmt.Errorf("unsupported memory index version %d", index.Version)
	}
	return validateEntries(scope, workspaceKey, index.Entries)
}

func (s *FileStore) writeUnlocked(ctx context.Context, dir string, entries []memory.Entry) (writeErr error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create memory directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect memory directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".index-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary memory index: %w", err)
	}
	temporaryPath := file.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil && writeErr == nil {
				writeErr = fmt.Errorf("close temporary memory index: %w", closeErr)
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && writeErr == nil {
			writeErr = fmt.Errorf("remove temporary memory index: %w", removeErr)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect temporary memory index: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(indexFile{Version: schemaVersion, Entries: entries}); err != nil {
		return fmt.Errorf("encode memory index: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync memory index: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close memory index: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before installing memory index: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(dir, "index.json")); err != nil {
		return fmt.Errorf("install memory index: %w", err)
	}
	return nil
}

func (s *FileStore) withScopeLock(ctx context.Context, dir string, fn func() error) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create memory store: %w", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return fmt.Errorf("protect memory store: %w", err)
	}
	lockPath := dir + ".lock"
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
			return fmt.Errorf("acquire memory lock: %w", err)
		}
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > lockStaleAfter {
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
