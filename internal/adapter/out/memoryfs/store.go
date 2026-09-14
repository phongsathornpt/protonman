// Package memoryfs implements durable memory persistence below ~/.protonman/memory.
package memoryfs

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

const schemaVersion = 1

type indexFile struct {
	Version int            `json:"version"`
	Entries []memory.Entry `json:"entries"`
	// Forgotten records IDs removed by Forget. Extraction regenerates an entry
	// deterministically whenever its stable ID is absent, so a deletion is only
	// durable if the ID is remembered and re-suppressed on every later merge.
	Forgotten []string `json:"forgotten,omitempty"`
}

type FileStore struct {
	root     string
	revision atomic.Uint64
}

var (
	_ memory.Repository     = (*FileStore)(nil)
	_ memory.RevisionSource = (*FileStore)(nil)
)

// Revision reports a counter that changes on every index write. Consumers that
// cache a retrieval read use it to notice a correction such as Forget.
func (s *FileStore) Revision() uint64 { return s.revision.Load() }

func NewFileStore(root string) (*FileStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("memory store root is required")
	}
	return &FileStore{root: root}, nil
}

func (s *FileStore) Load(ctx context.Context, scope memory.Scope, workspaceKey string) ([]memory.Entry, error) {
	dir, err := s.scopeDir(scope, workspaceKey)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("before loading memory: %w", err)
	}
	entries, forgotten, err := s.loadUnlocked(dir, scope, workspaceKey)
	if err != nil {
		return nil, err
	}
	// Tombstoned IDs are absent from the index, but defense in depth keeps a
	// forgotten memory from resurfacing if a legacy file still lists it.
	return filterForgotten(entries, forgottenSet(forgotten)), nil
}

func (s *FileStore) Replace(ctx context.Context, scope memory.Scope, workspaceKey string, entries []memory.Entry) error {
	dir, err := s.scopeDir(scope, workspaceKey)
	if err != nil {
		return err
	}
	prepared, err := validateEntries(scope, workspaceKey, entries)
	if err != nil {
		return err
	}
	return s.withScopeLock(ctx, dir, func() error {
		_, forgotten, err := s.loadUnlocked(dir, scope, workspaceKey)
		if err != nil {
			return err
		}
		return s.writeUnlocked(ctx, dir, filterForgotten(prepared, forgottenSet(forgotten)), forgotten)
	})
}

func (s *FileStore) Update(ctx context.Context, scope memory.Scope, workspaceKey string, update memory.UpdateFunc) error {
	if update == nil {
		return nil
	}
	dir, err := s.scopeDir(scope, workspaceKey)
	if err != nil {
		return err
	}
	return s.withScopeLock(ctx, dir, func() error {
		entries, forgotten, err := s.loadUnlocked(dir, scope, workspaceKey)
		if err != nil {
			return err
		}
		next, err := update(append([]memory.Entry(nil), entries...))
		if err != nil {
			return err
		}
		// A merge must never reintroduce a tombstoned memory.
		prepared, err := validateEntries(scope, workspaceKey, filterForgotten(next, forgottenSet(forgotten)))
		if err != nil {
			return err
		}
		return s.writeUnlocked(ctx, dir, prepared, forgotten)
	})
}

func (s *FileStore) RecordUsage(ctx context.Context, refs []memory.UsageRef, at time.Time) error {
	return s.recordUsage(ctx, refs, at)
}
