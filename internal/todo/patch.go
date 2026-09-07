package todo

import (
	"fmt"
	"strings"
)

type PatchOp string

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

func ApplyPatch(items []Item, operations []Operation) ([]Item, error) {
	if len(operations) == 0 {
		return nil, fmt.Errorf("todo patch requires at least one operation")
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
			item := Item{ID: operation.ID, Text: operation.Text, Status: operation.Status}
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
			next[position].Text = operation.Text
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
