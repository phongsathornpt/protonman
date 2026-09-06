package todo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// DefaultFilename is the workspace task file managed by Proton.
const DefaultFilename = "TODO.md"

type Status string

var (
	ErrRevisionConflict = errors.New("todo revision conflict")
	validIDPattern      = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
)

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

type Item struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status Status `json:"status"`
}

type Snapshot struct {
	Revision uint64 `json:"revision"`
	Items    []Item `json:"items"`
}

func (s Status) Valid() bool {
	return s == StatusPending || s == StatusInProgress || s == StatusCompleted
}

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
		if strings.ContainsAny(item.Text, "\r\n") || strings.Contains(item.Text, managedStart) || strings.Contains(item.Text, managedEnd) {
			return fmt.Errorf("todo item %q: text must be a single safe markdown line", id)
		}
		if !item.Status.Valid() {
			return fmt.Errorf("todo item %q: invalid status %q", id, item.Status)
		}
	}
	return nil
}

func CloneItems(items []Item) []Item {
	if len(items) == 0 {
		return nil
	}
	out := make([]Item, len(items))
	copy(out, items)
	return out
}

func CloneSnapshot(s Snapshot) Snapshot { s.Items = CloneItems(s.Items); return s }

func LegacyID(text string, occurrence int) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return "legacy-" + hex.EncodeToString(h[:6]) + fmt.Sprintf("-%d", occurrence)
}
