package tui

import "strings"

// TodoItem is one entry shown in the fullscreen TODO panel.
type TodoItem struct {
	Text string
	Done bool
}

// ParseTODO extracts checkbox items from a Markdown TODO file.
func ParseTODO(markdown string) []TodoItem {
	items := make([]TodoItem, 0)
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		done := false
		switch {
		case strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "):
			done = true
			trimmed = strings.TrimSpace(trimmed[6:])
		case strings.HasPrefix(trimmed, "- [ ] "):
			trimmed = strings.TrimSpace(trimmed[6:])
		default:
			continue
		}
		if trimmed != "" {
			items = append(items, TodoItem{Text: trimmed, Done: done})
		}
	}
	return items
}
