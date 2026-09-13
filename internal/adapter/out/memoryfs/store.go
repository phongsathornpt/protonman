// Package memoryfs implements durable memory persistence below ~/.protonman/memory.
package memoryfs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

const schemaVersion = 1

type indexFile struct {
	Version int            `json:"version"`
	Entries []memory.Entry `json:"entries"`
}

type FileStore struct{ root string }

var _ memory.Repository = (*FileStore)(nil)

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
	return s.loadUnlocked(dir, scope, workspaceKey)
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
		return s.writeUnlocked(ctx, dir, prepared)
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
		entries, err := s.loadUnlocked(dir, scope, workspaceKey)
		if err != nil {
			return err
		}
		next, err := update(append([]memory.Entry(nil), entries...))
		if err != nil {
			return err
		}
		prepared, err := validateEntries(scope, workspaceKey, next)
		if err != nil {
			return err
		}
		return s.writeUnlocked(ctx, dir, prepared)
	})
}

func (s *FileStore) RecordUsage(ctx context.Context, refs []memory.UsageRef, at time.Time) error {
	return s.recordUsage(ctx, refs, at)
}
