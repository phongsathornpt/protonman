package memoryfs

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

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

const lockStaleAfter = 2 * time.Minute

func (s *FileStore) loadUnlocked(dir string, scope memory.Scope, workspaceKey string) ([]memory.Entry, []string, error) {
	file, err := os.Open(filepath.Join(dir, "index.json"))
	if errors.Is(err, os.ErrNotExist) {
		return []memory.Entry{}, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open memory index: %w", err)
	}
	var index indexFile
	decodeErr := json.NewDecoder(file).Decode(&index)
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, nil, fmt.Errorf("decode memory index: %w", decodeErr)
	}
	if closeErr != nil {
		return nil, nil, fmt.Errorf("close memory index: %w", closeErr)
	}
	if index.Version != schemaVersion {
		return nil, nil, fmt.Errorf("unsupported memory index version %d", index.Version)
	}
	entries, err := validateEntries(scope, workspaceKey, index.Entries)
	if err != nil {
		return nil, nil, err
	}
	return entries, normalizeForgotten(index.Forgotten), nil
}

// normalizeForgotten returns the tombstone set de-duplicated in insertion order.
// Order is preserved (not sorted) so that when the set exceeds its bound the
// oldest forgets are evicted first rather than an arbitrary ID.
func normalizeForgotten(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func forgottenSet(ids []string) map[string]struct{} {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// filterForgotten drops entries whose ID is tombstoned.
func filterForgotten(entries []memory.Entry, set map[string]struct{}) []memory.Entry {
	if len(set) == 0 {
		return entries
	}
	kept := make([]memory.Entry, 0, len(entries))
	for _, entry := range entries {
		if _, drop := set[entry.ID]; drop {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// mergeForgotten appends newly forgotten IDs to the existing ordered tombstone
// set. Tombstones are retained indefinitely because Forget is a durable user
// correction; eviction would allow extraction to resurrect the entry.
func mergeForgotten(existing []string, added map[string]struct{}) []string {
	unique := normalizeForgotten(existing)
	if len(added) == 0 {
		return unique
	}
	present := make(map[string]struct{}, len(unique))
	for _, id := range unique {
		present[id] = struct{}{}
	}
	// Sort the additions for determinism, then append.
	newIDs := make([]string, 0, len(added))
	for id := range added {
		newIDs = append(newIDs, id)
	}
	sort.Strings(newIDs)
	for _, id := range newIDs {
		if _, ok := present[id]; ok {
			continue
		}
		present[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func (s *FileStore) writeUnlocked(ctx context.Context, dir string, entries []memory.Entry, forgotten []string) (writeErr error) {
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
	if err := encoder.Encode(indexFile{Version: schemaVersion, Entries: entries, Forgotten: normalizeForgotten(forgotten)}); err != nil {
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
	// Publish a new revision only after the write is durable, so a consumer that
	// observes the change is guaranteed to read the committed index.
	s.revision.Add(1)
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
