package todo

import (
	"context"
	"testing"
)

func TestReconcileStatusUpdatesExistingTaskAndIgnoresMissing(t *testing.T) {
	store, err := NewStore([]Item{{ID: "build", Text: "build it", Status: StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, changed, err := ReconcileStatus(context.Background(), store, "build", StatusInProgress)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || snapshot.Items[0].Status != StatusInProgress {
		t.Fatalf("snapshot=%+v changed=%v", snapshot, changed)
	}
	snapshot, changed, err = ReconcileStatus(context.Background(), store, "missing", StatusCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if changed || len(snapshot.Items) != 1 {
		t.Fatalf("missing task reconciliation changed plan: %+v", snapshot)
	}
}
