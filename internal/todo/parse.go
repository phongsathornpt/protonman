package todo

import "strings"

func ParseMarkdown(markdown string) []Item {
	items := make([]Item, 0)
	occurrences := map[string]int{}
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		status := StatusPending
		switch {
		case strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "):
			status = StatusCompleted
			trimmed = strings.TrimSpace(trimmed[6:])
		case strings.HasPrefix(trimmed, "- [~] "):
			status = StatusInProgress
			trimmed = strings.TrimSpace(trimmed[6:])
		case strings.HasPrefix(trimmed, "- [ ] "):
			trimmed = strings.TrimSpace(trimmed[6:])
		default:
			continue
		}
		if trimmed == "" {
			continue
		}
		occurrences[trimmed]++
		items = append(items, Item{ID: LegacyID(trimmed, occurrences[trimmed]), Text: trimmed, Status: status})
	}
	return items
}
