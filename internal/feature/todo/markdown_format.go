package todo

import (
	"fmt"
	"strings"
)

func parseDocument(content string) ([]Item, error) {
	startCount := strings.Count(content, managedStart)
	endCount := strings.Count(content, managedEnd)
	if startCount == 0 && endCount == 0 {
		return ParseMarkdown(content), nil
	}
	if startCount != 1 || endCount != 1 {
		return nil, fmt.Errorf("invalid proton todo managed section count")
	}
	start := strings.Index(content, managedStart)
	end := strings.Index(content, managedEnd)
	if start < 0 || end < 0 || end < start {
		return nil, fmt.Errorf("invalid proton todo managed section")
	}
	body := content[start+len(managedStart) : end]
	items := make([]Item, 0)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		status := StatusPending
		switch {
		case strings.HasPrefix(trimmed, "- [ ] "):
		case strings.HasPrefix(trimmed, "- [~] "):
			status = StatusInProgress
		case strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "):
			status = StatusCompleted
		default:
			continue
		}
		rest := strings.TrimSpace(trimmed[6:])
		if !strings.HasPrefix(rest, "[") {
			return nil, fmt.Errorf("managed todo item missing id: %q", trimmed)
		}
		close := strings.IndexByte(rest, ']')
		if close <= 1 {
			return nil, fmt.Errorf("managed todo item missing id: %q", trimmed)
		}
		items = append(items, Item{ID: rest[1:close], Text: strings.TrimSpace(rest[close+1:]), Status: status})
	}
	if err := ValidateItems(items); err != nil {
		return nil, err
	}
	return items, nil
}

func renderDocument(content string, items []Item) string {
	section := renderManaged(items)
	start := strings.Index(content, managedStart)
	end := strings.Index(content, managedEnd)
	if start >= 0 && end >= start {
		end += len(managedEnd)
		return content[:start] + section + content[end:]
	}
	if content == "" {
		return section + "\n"
	}
	sep := "\n"
	if strings.HasSuffix(content, "\n") {
		sep = ""
	}
	return content + sep + "\n" + section + "\n"
}

func renderManaged(items []Item) string {
	lines := []string{managedStart}
	for _, item := range items {
		mark := " "
		if item.Status == StatusInProgress {
			mark = "~"
		}
		if item.Status == StatusCompleted {
			mark = "x"
		}
		lines = append(lines, fmt.Sprintf("- [%s] [%s] %s", mark, item.ID, item.Text))
	}
	lines = append(lines, managedEnd)
	return strings.Join(lines, "\n")
}
