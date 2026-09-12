package todo

import (
	"fmt"
	"strings"
)

type PatchOp string
type PatchImpact string

const (
	PatchImpactUnknown    PatchImpact = ""
	PatchImpactStatusOnly PatchImpact = "status_only"
	PatchImpactStructural PatchImpact = "structural"
)

const (
	PatchAdd       PatchOp = "add"
	PatchSetStatus PatchOp = "set_status"
	PatchSetText   PatchOp = "set_text"
	PatchRemove    PatchOp = "remove"
)

type Operation struct {
	Op     PatchOp `json:"op"`
	ID     string  `json:"id"`
	Text   string  `json:"text,omitempty"`
	Status Status  `json:"status,omitempty"`
}

type PatchEffects struct {
	Structural bool
	Text       bool
	Status     bool
	Valid      bool
}

func EffectsOfPatch(operations []Operation) PatchEffects {
	if len(operations) == 0 {
		return PatchEffects{}
	}
	effects := PatchEffects{Valid: true}
	for _, operation := range operations {
		switch operation.Op {
		case PatchAdd, PatchRemove:
			effects.Structural = true
		case PatchSetText:
			effects.Text = true
		case PatchSetStatus:
			effects.Status = true
		default:
			return PatchEffects{}
		}
	}
	return effects
}

func ClassifyPatch(operations []Operation) PatchImpact {
	effects := EffectsOfPatch(operations)
	if !effects.Valid {
		return PatchImpactUnknown
	}
	if effects.Structural || effects.Text {
		return PatchImpactStructural
	}
	if effects.Status {
		return PatchImpactStatusOnly
	}
	return PatchImpactUnknown
}

func validatePatchWrites(operations []Operation) error {
	type writes struct{ structural, text, status bool }
	seen := make(map[string]writes, len(operations))
	for index, operation := range operations {
		id := strings.TrimSpace(operation.ID)
		current := seen[id]
		if current.structural {
			return fmt.Errorf("todo operation %d: id %q already has a structural write in this patch", index, id)
		}
		switch operation.Op {
		case PatchAdd, PatchRemove:
			if current.text || current.status {
				return fmt.Errorf("todo operation %d: structural write for id %q conflicts with earlier field writes", index, id)
			}
			current.structural = true
		case PatchSetText:
			if current.text {
				return fmt.Errorf("todo operation %d: duplicate text write for id %q", index, id)
			}
			current.text = true
		case PatchSetStatus:
			if current.status {
				return fmt.Errorf("todo operation %d: duplicate status write for id %q", index, id)
			}
			current.status = true
		}
		seen[id] = current
	}
	return nil
}

func ApplyPatch(items []Item, operations []Operation) ([]Item, error) {
	if len(operations) == 0 {
		return nil, fmt.Errorf("todo patch requires at least one operation")
	}
	if err := validatePatchWrites(operations); err != nil {
		return nil, err
	}
	next := CloneItems(items)
	if err := ValidateItems(next); err != nil {
		return nil, fmt.Errorf("validate current todo items: %w", err)
	}
	for index, operation := range operations {
		operation.ID = strings.TrimSpace(operation.ID)
		if operation.ID == "" || !validIDPattern.MatchString(operation.ID) {
			return nil, fmt.Errorf("todo operation %d: invalid id %q", index, operation.ID)
		}
		position := findItem(next, operation.ID)
		switch operation.Op {
		case PatchAdd:
			if position >= 0 {
				return nil, fmt.Errorf("todo operation %d: id %q already exists", index, operation.ID)
			}
			item := Item{ID: operation.ID, Text: strings.TrimSpace(operation.Text), Status: operation.Status}
			if err := ValidateItems([]Item{item}); err != nil {
				return nil, fmt.Errorf("todo operation %d: %w", index, err)
			}
			next = append(next, item)
		case PatchSetStatus:
			if position < 0 {
				return nil, fmt.Errorf("todo operation %d: id %q does not exist", index, operation.ID)
			}
			if !operation.Status.Valid() {
				return nil, fmt.Errorf("todo operation %d: invalid status %q", index, operation.Status)
			}
			if strings.TrimSpace(operation.Text) != "" {
				return nil, fmt.Errorf("todo operation %d: set_status does not accept text", index)
			}
			next[position].Status = operation.Status
		case PatchSetText:
			if position < 0 {
				return nil, fmt.Errorf("todo operation %d: id %q does not exist", index, operation.ID)
			}
			if operation.Status != "" {
				return nil, fmt.Errorf("todo operation %d: set_text does not accept status", index)
			}
			next[position].Text = strings.TrimSpace(operation.Text)
			if err := ValidateItems(next); err != nil {
				return nil, fmt.Errorf("todo operation %d: %w", index, err)
			}
		case PatchRemove:
			if position < 0 {
				return nil, fmt.Errorf("todo operation %d: id %q does not exist", index, operation.ID)
			}
			if strings.TrimSpace(operation.Text) != "" || operation.Status != "" {
				return nil, fmt.Errorf("todo operation %d: remove accepts only id", index)
			}
			next = append(next[:position], next[position+1:]...)
		default:
			return nil, fmt.Errorf("todo operation %d: unsupported op %q", index, operation.Op)
		}
	}
	if err := ValidateItems(next); err != nil {
		return nil, err
	}
	return next, nil
}

func findItem(items []Item, id string) int {
	for index, item := range items {
		if item.ID == id {
			return index
		}
	}
	return -1
}
