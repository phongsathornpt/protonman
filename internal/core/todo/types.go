// Package todo owns the pure task-plan domain contracts: task items,
// snapshots, patch vocabulary, and the repository ports used by tools,
// adapters, and the turn engine. Filesystem persistence lives in
// internal/feature/todo; this package stays implementation-free so the
// application layer can depend on it without reaching feature packages.
package todo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Status is the lifecycle state of one task item.
type Status string

// ErrRevisionConflict signals an optimistic-concurrency failure.
var ErrRevisionConflict = errors.New("todo revision conflict")

const (
	// StatusPending marks a task not yet started.
	StatusPending Status = "pending"
	// StatusInProgress marks a task currently being worked on.
	StatusInProgress Status = "in_progress"
	// StatusCompleted marks a finished task.
	StatusCompleted Status = "completed"
)

// Item is one durable task-plan entry.
type Item struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status Status `json:"status"`
}

// Snapshot is a versioned view of the full task plan.
type Snapshot struct {
	Revision uint64 `json:"revision"`
	Items    []Item `json:"items"`
}

var validIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// ValidID reports whether the id matches the durable task-id vocabulary.
func ValidID(id string) bool { return validIDPattern.MatchString(id) }

// Valid reports whether the status is one of the canonical values.
func (s Status) Valid() bool {
	return s == StatusPending || s == StatusInProgress || s == StatusCompleted
}

// ValidateItems enforces the durable task-plan invariants: unique safe ids,
// single-line safe text, and canonical statuses.
func ValidateItems(items []Item) error {
	seen := make(map[string]struct{}, len(items))
	for idx, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("todo item %d: id is required", idx)
		}
		if !validIDPattern.MatchString(id) {
			return fmt.Errorf("todo item %d: id %q must match %s", idx, id, validIDPattern.String())
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("todo item %d: duplicate id %q", idx, id)
		}
		seen[id] = struct{}{}
		text := strings.TrimSpace(item.Text)
		if text == "" {
			return fmt.Errorf("todo item %q: text is required", id)
		}
		if strings.ContainsAny(item.Text, "\r\n") || strings.Contains(item.Text, "<!-- proton:") {
			return fmt.Errorf("todo item %q: text must be a single safe markdown line", id)
		}
		if !item.Status.Valid() {
			return fmt.Errorf("todo item %q: invalid status %q", id, item.Status)
		}
	}
	return nil
}

// ActiveItems returns every task currently marked in progress. Multiple active
// items are intentional: root and delegated work may execute concurrently.
// Execution ownership is runtime state and is deliberately not persisted in TODO.md.
func ActiveItems(items []Item) []Item {
	out := make([]Item, 0)
	for _, item := range items {
		if item.Status == StatusInProgress {
			out = append(out, item)
		}
	}
	return out
}

// CloneItems returns a defensive copy of the items.
func CloneItems(items []Item) []Item {
	out := make([]Item, len(items))
	copy(out, items)
	return out
}

// CloneSnapshot returns a defensive copy of the snapshot.
func CloneSnapshot(s Snapshot) Snapshot { s.Items = CloneItems(s.Items); return s }

// LegacyID derives a stable id for plans persisted before ids were stored.
func LegacyID(text string, occurrence int) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return "legacy-" + hex.EncodeToString(h[:6]) + fmt.Sprintf("-%d", occurrence)
}
