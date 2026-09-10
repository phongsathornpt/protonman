package todo

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

// Store owns the current task snapshot and serializes task mutations.
type Store struct {
	mu       sync.RWMutex
	revision uint64
	items    []Item
}

func NewStore(initial []Item) (*Store, error) {
	if err := ValidateItems(initial); err != nil {
		return nil, err
	}
	return &Store{items: CloneItems(initial)}, nil
}

func newStoreWithRevision(revision uint64, initial []Item) (*Store, error) {
	if err := ValidateItems(initial); err != nil {
		return nil, err
	}
	return &Store{revision: revision, items: CloneItems(initial)}, nil
}

func (s *Store) setSnapshot(revision uint64, items []Item) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revision = revision
	s.items = CloneItems(items)
	return Snapshot{Revision: s.revision, Items: CloneItems(s.items)}
}

func (s *Store) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{Revision: s.revision, Items: CloneItems(s.items)}
}

func (s *Store) Replace(ctx context.Context, items []Item) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := ValidateItems(items); err != nil {
		return Snapshot{}, err
	}
	next := CloneItems(items)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if !slices.Equal(s.items, next) {
		s.items = next
		s.revision++
	}
	return Snapshot{Revision: s.revision, Items: CloneItems(s.items)}, nil
}

func (s *Store) CompareAndPatch(ctx context.Context, expectedRevision uint64, operations []Operation) (Snapshot, Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, Snapshot{}, err
	}
	before := Snapshot{Revision: s.revision, Items: CloneItems(s.items)}
	if s.revision != expectedRevision {
		return before, before, fmt.Errorf("%w: expected %d, current %d", ErrRevisionConflict, expectedRevision, s.revision)
	}
	next, err := ApplyPatch(s.items, operations)
	if err != nil {
		return before, before, err
	}
	if !slices.Equal(s.items, next) {
		s.items = CloneItems(next)
		s.revision++
	}
	after := Snapshot{Revision: s.revision, Items: CloneItems(s.items)}
	return before, after, nil
}

func (s *Store) CompareAndReplace(ctx context.Context, expectedRevision uint64, items []Item) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := ValidateItems(items); err != nil {
		return Snapshot{}, err
	}
	next := CloneItems(items)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if s.revision != expectedRevision {
		return Snapshot{Revision: s.revision, Items: CloneItems(s.items)}, fmt.Errorf("%w: expected %d, current %d", ErrRevisionConflict, expectedRevision, s.revision)
	}
	if !slices.Equal(s.items, next) {
		s.items = next
		s.revision++
	}
	return Snapshot{Revision: s.revision, Items: CloneItems(s.items)}, nil
}
