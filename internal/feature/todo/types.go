// Package todo implements durable, session-owned task plans (TODO.md) with
// optimistic concurrency and active-goal binding. The pure domain contracts —
// items, snapshots, patch vocabulary, and repository ports — live in
// internal/core/todo; the aliases below preserve the package-local names for
// this implementation and for existing consumers. Do not add new domain policy
// here; extend internal/core/todo instead.
package todo

import (
	tododomain "github.com/phongsathornpt/protonman/internal/core/todo"
)

type (
	// Status is the lifecycle state of one task item (core contract).
	Status = tododomain.Status
	// Item is one durable task-plan entry (core contract).
	Item = tododomain.Item
	// Snapshot is a versioned view of the full task plan (core contract).
	Snapshot = tododomain.Snapshot
	// Operation is one validated task-plan mutation (core contract).
	Operation = tododomain.Operation
	// PatchEffects summarizes which plan surfaces a validated patch touches.
	PatchEffects = tododomain.PatchEffects
	// PatchOp enumerates the supported task-plan patch operations.
	PatchOp = tododomain.PatchOp
	// PatchImpact classifies the blast radius of a patch after validation.
	PatchImpact = tododomain.PatchImpact
)

const (
	// StatusPending marks a task not yet started.
	StatusPending = tododomain.StatusPending
	// StatusInProgress marks a task currently being worked on.
	StatusInProgress = tododomain.StatusInProgress
	// StatusCompleted marks a finished task.
	StatusCompleted = tododomain.StatusCompleted

	// PatchImpactUnknown marks an invalid or empty patch.
	PatchImpactUnknown = tododomain.PatchImpactUnknown
	// PatchImpactStatusOnly marks a patch that only flips task statuses.
	PatchImpactStatusOnly = tododomain.PatchImpactStatusOnly
	// PatchImpactStructural marks a patch that adds/removes tasks or rewrites text.
	PatchImpactStructural = tododomain.PatchImpactStructural

	// PatchAdd appends a new task.
	PatchAdd = tododomain.PatchAdd
	// PatchSetStatus flips an existing task's status.
	PatchSetStatus = tododomain.PatchSetStatus
	// PatchSetText rewrites an existing task's text.
	PatchSetText = tododomain.PatchSetText
	// PatchRemove deletes an existing task.
	PatchRemove = tododomain.PatchRemove
)

// ErrRevisionConflict signals an optimistic-concurrency failure (core contract).
var ErrRevisionConflict = tododomain.ErrRevisionConflict

// ValidID reports whether the id matches the durable task-id vocabulary.
func ValidID(id string) bool { return tododomain.ValidID(id) }

// ValidateItems enforces the durable task-plan invariants.
func ValidateItems(items []Item) error { return tododomain.ValidateItems(items) }

// ActiveItems returns every task currently marked in progress.
func ActiveItems(items []Item) []Item { return tododomain.ActiveItems(items) }

// CloneItems returns a defensive copy of the items.
func CloneItems(items []Item) []Item { return tododomain.CloneItems(items) }

// CloneSnapshot returns a defensive copy of the snapshot.
func CloneSnapshot(s Snapshot) Snapshot { return tododomain.CloneSnapshot(s) }

// LegacyID derives a stable id for plans persisted before ids were stored.
func LegacyID(text string, occurrence int) string { return tododomain.LegacyID(text, occurrence) }

// EffectsOfPatch validates the operation vocabulary and classifies the effects.
func EffectsOfPatch(operations []Operation) PatchEffects {
	return tododomain.EffectsOfPatch(operations)
}

// ClassifyPatch maps validated effects onto the coarse patch-impact policy.
func ClassifyPatch(operations []Operation) PatchImpact {
	return tododomain.ClassifyPatch(operations)
}
