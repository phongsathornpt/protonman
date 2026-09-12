package todo

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const reconcileStatusAttempts = 3

// ReconcileStatus applies a runtime-owned task status transition when the task
// still exists. Revision conflicts are retried against the repository's refreshed
// projection so lifecycle events do not blindly overwrite concurrent plan edits.
func ReconcileStatus(ctx context.Context, repo PatchRepository, id string, status Status) (Snapshot, bool, error) {
	if repo == nil {
		return Snapshot{}, false, fmt.Errorf("todo repository is required")
	}
	id = strings.TrimSpace(id)
	if !validIDPattern.MatchString(id) {
		return repo.Snapshot(), false, fmt.Errorf("invalid todo id %q", id)
	}
	if !status.Valid() {
		return repo.Snapshot(), false, fmt.Errorf("invalid todo status %q", status)
	}
	for attempt := 0; attempt < reconcileStatusAttempts; attempt++ {
		snapshot := repo.Snapshot()
		position := findItem(snapshot.Items, id)
		if position < 0 {
			return snapshot, false, nil
		}
		if snapshot.Items[position].Status == status {
			return snapshot, false, nil
		}
		_, after, err := repo.CompareAndPatch(ctx, snapshot.Revision, []Operation{{Op: PatchSetStatus, ID: id, Status: status}})
		if err == nil {
			return after, true, nil
		}
		if !errors.Is(err, ErrRevisionConflict) {
			return after, false, err
		}
	}
	return repo.Snapshot(), false, ErrRevisionConflict
}
