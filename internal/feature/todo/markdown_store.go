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
	var out Snapshot
	err := withFileLock(ctx, s.path, func() error {
		revision, items, _, err := readMarkdownSnapshot(s.path)
		if err != nil {
			return err
		}
		out = s.mem.setSnapshot(revision, items)
		return nil
	})
	return out, err
}

func (s *MarkdownStore) BindGoal(ctx context.Context, goal string) (Snapshot, bool, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	target := goalFingerprint(goal)
	var out Snapshot
	changed := false
	err := withFileLock(ctx, s.path, func() error {
		revision, diskItems, content, err := readMarkdownSnapshot(s.path)
		if err != nil {
			return err
		}
		current, present, err := parseGoalBinding(content)
		if err != nil {
			return err
		}
		if present && current == target {
			out = s.mem.setSnapshot(revision, diskItems)
			return nil
		}
		if !present && target == goalUnboundMarker {
			// Preserve legacy/unbound plans while no active goal exists so the
			// first non-empty goal can adopt them instead of treating them as stale.
			out = s.mem.setSnapshot(revision, diskItems)
			return nil
		}
		nextItems := CloneItems(diskItems)
		if present && target != goalUnboundMarker && current != target {
			nextItems = nil
		}
		nextRevision := revision + 1
		next := renderGoalBinding(renderDocumentState(content, nextRevision, nextItems), target)
		if err := writeAtomic(ctx, s.path, []byte(next)); err != nil {
			return err
		}
		out = s.mem.setSnapshot(nextRevision, nextItems)
		changed = true
		return nil
	})
	return out, changed, err
}

func (s *MarkdownStore) CompareAndPatch(ctx context.Context, expectedRevision uint64, operations []Operation) (Snapshot, Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var before, after Snapshot
	err := withFileLock(ctx, s.path, func() error {
		revision, diskItems, content, err := readMarkdownSnapshot(s.path)
		if err != nil {
			return err
		}
		before = Snapshot{Revision: revision, Items: CloneItems(diskItems)}
		if revision != expectedRevision {
			after = s.mem.setSnapshot(revision, diskItems)
			return fmt.Errorf("%w: expected %d, current %d", ErrRevisionConflict, expectedRevision, revision)
		}
		next, err := ApplyPatch(diskItems, operations)
		if err != nil {
			return err
		}
		if itemsEqual(diskItems, next) {
			after = s.mem.setSnapshot(revision, diskItems)
			return nil
		}
		nextRevision := revision + 1
		nextContent := renderDocumentState(content, nextRevision, next)
		if err := writeAtomic(ctx, s.path, []byte(nextContent)); err != nil {
			return err
		}
		after = s.mem.setSnapshot(nextRevision, next)
		return nil
	})
	return before, after, err
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
	if _, _, err := parseGoalBinding(string(content)); err != nil {
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
