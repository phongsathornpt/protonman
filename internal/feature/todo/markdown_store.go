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
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read todo markdown: %w", err)
	}
	items, err := parseDocument(string(content))
	if err != nil {
		return nil, err
	}
	mem, err := NewStore(items)
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
	current := s.mem.Snapshot()
	if itemsEqual(current.Items, items) {
		return current, nil
	}
	content, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, fmt.Errorf("read todo markdown: %w", err)
	}
	next := renderDocument(string(content), items)
	if err := writeAtomic(ctx, s.path, []byte(next)); err != nil {
		return Snapshot{}, err
	}
	return s.mem.Replace(ctx, items)
}

func (s *MarkdownStore) Reload(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	content, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		content = nil
	} else if err != nil {
		return Snapshot{}, fmt.Errorf("read todo markdown: %w", err)
	}
	items, err := parseDocument(string(content))
	if err != nil {
		return Snapshot{}, err
	}
	return s.mem.Replace(ctx, items)
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

func (s *MarkdownStore) CompareAndReplace(ctx context.Context, expectedRevision uint64, items []Item) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := ValidateItems(items); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.mem.Snapshot()
	if current.Revision != expectedRevision {
		return current, fmt.Errorf("%w: expected %d, current %d", ErrRevisionConflict, expectedRevision, current.Revision)
	}
	if itemsEqual(current.Items, items) {
		return current, nil
	}
	content, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, fmt.Errorf("read todo markdown: %w", err)
	}
	next := renderDocument(string(content), items)
	if err := writeAtomic(ctx, s.path, []byte(next)); err != nil {
		return Snapshot{}, err
	}
	return s.mem.CompareAndReplace(ctx, expectedRevision, items)
}
