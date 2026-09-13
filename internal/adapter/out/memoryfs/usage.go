package memoryfs

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

func (s *FileStore) recordUsage(ctx context.Context, refs []memory.UsageRef, at time.Time) error {
	if len(refs) == 0 {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	type group struct {
		scope        memory.Scope
		workspaceKey string
		ids          map[string]struct{}
	}
	groups := make(map[string]*group)
	for _, ref := range refs {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			continue
		}
		dir, err := s.scopeDir(ref.Scope, ref.WorkspaceKey)
		if err != nil {
			return err
		}
		g := groups[dir]
		if g == nil {
			g = &group{scope: ref.Scope, workspaceKey: strings.TrimSpace(ref.WorkspaceKey), ids: make(map[string]struct{})}
			groups[dir] = g
		}
		g.ids[id] = struct{}{}
	}
	dirs := make([]string, 0, len(groups))
	for dir := range groups {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		g := groups[dir]
		if err := s.withScopeLock(ctx, dir, func() error {
			entries, err := s.loadUnlocked(dir, g.scope, g.workspaceKey)
			if err != nil {
				return err
			}
			changed := false
			for i := range entries {
				if _, ok := g.ids[entries[i].ID]; !ok {
					continue
				}
				entries[i].UsageCount++
				entries[i].LastUsedAt = at
				changed = true
			}
			if !changed {
				return nil
			}
			return s.writeUnlocked(ctx, dir, entries)
		}); err != nil {
			return err
		}
	}
	return nil
}
