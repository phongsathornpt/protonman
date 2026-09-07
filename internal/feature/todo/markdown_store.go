package todo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
)

const (
	managedStart = "<!-- proton:todos:start -->"
	managedEnd   = "<!-- proton:todos:end -->"
)

type MarkdownStore struct {
	mu   sync.Mutex
	path string
	mem  *Store
}

func OpenMarkdownStore(ctx context.Context, path string) (*MarkdownStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	revision, items, _, err := readMarkdownSnapshot(path)
	if err != nil {
		return nil, err
	}
	mem, err := newStoreWithRevision(revision, items)
	if err != nil {
		return nil, err
	}
	return &MarkdownStore{path: path, mem: mem}, nil
}

func (s *MarkdownStore) Snapshot() Snapshot {
	if s == nil || s.mem == nil {
		return Snapshot{}
	}
	return s.mem.Snapshot()
}

func (s *MarkdownStore) Replace(ctx context.Context, items []Item) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := ValidateItems(items); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Snapshot
	err := withFileLock(ctx, s.path, func() error {
		revision, diskItems, content, err := readMarkdownSnapshot(s.path)
		if err != nil {
			return err
		}
		if itemsEqual(diskItems, items) {
			out = s.mem.setSnapshot(revision, diskItems)
			return nil
		}
		nextRevision := revision + 1
		next := renderDocumentState(content, nextRevision, items)
		if err := writeAtomic(ctx, s.path, []byte(next)); err != nil {
			return err
		}
		out = s.mem.setSnapshot(nextRevision, items)
		return nil
	})
	return out, err
}

func (s *MarkdownStore) Reload(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	revision, items, _, err := readMarkdownSnapshot(s.path)
	if err != nil {
		return Snapshot{}, err
	}
	return s.mem.setSnapshot(revision, items), nil
}

func (s *MarkdownStore) CompareAndReplace(ctx context.Context, expectedRevision uint64, items []Item) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := ValidateItems(items); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Snapshot
	err := withFileLock(ctx, s.path, func() error {
		revision, diskItems, content, err := readMarkdownSnapshot(s.path)
		if err != nil {
			return err
		}
		if revision != expectedRevision {
			out = s.mem.setSnapshot(revision, diskItems)
			return fmt.Errorf("%w: expected %d, current %d", ErrRevisionConflict, expectedRevision, revision)
		}
		if itemsEqual(diskItems, items) {
			out = s.mem.setSnapshot(revision, diskItems)
			return nil
		}
		nextRevision := revision + 1
		next := renderDocumentState(content, nextRevision, items)
		if err := writeAtomic(ctx, s.path, []byte(next)); err != nil {
			return err
		}
		out = s.mem.setSnapshot(nextRevision, items)
		return nil
	})
	return out, err
}

func readMarkdownSnapshot(path string) (uint64, []Item, string, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		content = nil
	} else if err != nil {
		return 0, nil, "", fmt.Errorf("read todo markdown: %w", err)
	}
	revision, items, err := parseDocumentState(string(content))
	if err != nil {
		return 0, nil, "", err
	}
	return revision, items, string(content), nil
}

func itemsEqual(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
