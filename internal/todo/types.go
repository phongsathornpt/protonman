package todo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Status string

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

func (i Item) EffectiveStatus() Status {
	if i.Status == "" {
		return StatusPending
	}
	return i.Status
}

func (i Item) Done() bool { return i.EffectiveStatus() == StatusCompleted }

func ValidateItems(items []Item) error {
	seen := make(map[string]struct{}, len(items))
	for idx, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("todo item %d: id is required", idx)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("todo item %d: duplicate id %q", idx, id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(item.Text) == "" {
			return fmt.Errorf("todo item %q: text is required", id)
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
