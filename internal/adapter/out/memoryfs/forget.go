package memoryfs

import (
	"context"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

// Forget permanently removes entries by ID from one scope index and records a
// tombstone for each ID.
//
// The tombstone is what makes forgetting durable. Extraction derives an entry's
// stable ID deterministically, so the next extraction pass would otherwise
// recreate any entry whose ID is simply missing from the index. Recording the ID
// as forgotten lets every later merge or usage write suppress it again.
//
// Forget reuses the scope write lock and atomic index replacement used by
// Replace/Update so a concurrent retrieval or extraction merge cannot interleave
// a partial delete. Unknown IDs are still tombstoned and reported as zero
// removals, which keeps a repeated request idempotent.
func (s *FileStore) Forget(ctx context.Context, scope memory.Scope, workspaceKey string, ids []string) (int, error) {
	dir, err := s.scopeDir(scope, workspaceKey)
	if err != nil {
		return 0, err
	}
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			unique[id] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return 0, nil
	}
	removed := 0
	err = s.withScopeLock(ctx, dir, func() error {
		entries, forgotten, err := s.loadUnlocked(dir, scope, workspaceKey)
		if err != nil {
			return err
		}
		kept := make([]memory.Entry, 0, len(entries))
		for _, entry := range entries {
			if _, drop := unique[entry.ID]; drop {
				removed++
				continue
			}
			kept = append(kept, entry)
		}
		// Always persist the tombstone, even when nothing was removed, so a
		// future extraction pass cannot recreate the entry.
		return s.writeUnlocked(ctx, dir, kept, mergeForgotten(forgotten, unique))
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}
