package pane

import tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"

func TodoCounts(items []tododomain.Item) (completed, active, pending int) {
	for _, item := range items {
		switch item.Status {
		case tododomain.StatusCompleted:
			completed++
		case tododomain.StatusInProgress:
			active++
		case tododomain.StatusPending:
			pending++
		}
	}
	return completed, active, pending
}
