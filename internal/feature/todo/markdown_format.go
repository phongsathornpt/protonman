package todo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const (
	revisionMarkerPrefix = "<!-- proton:todo version=1 revision="
	goalMarkerPrefix     = "<!-- proton:todo-goal sha256="
	goalUnboundMarker    = "none"
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

func parseDocumentState(content string) (uint64, []Item, error) {
	revision, err := parseRevision(content)
	if err != nil {
		return 0, nil, err
	}
	items, err := parseDocument(content)
	if err != nil {
		return 0, nil, err
	}
	return revision, items, nil
}

func parseRevision(content string) (uint64, error) {
	count := strings.Count(content, revisionMarkerPrefix)
	if count == 0 {
		return 0, nil
	}
	if count != 1 {
		return 0, fmt.Errorf("invalid proton todo revision marker count")
	}
	start := strings.Index(content, revisionMarkerPrefix)
	valueStart := start + len(revisionMarkerPrefix)
	end := strings.Index(content[valueStart:], " -->")
	if end < 0 {
		return 0, fmt.Errorf("invalid proton todo revision marker")
	}
	value := strings.TrimSpace(content[valueStart : valueStart+end])
	revision, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid proton todo revision %q: %w", value, err)
	}
	return revision, nil
}

func renderDocumentState(content string, revision uint64, items []Item) string {
	rendered := renderDocument(content, items)
	marker := fmt.Sprintf("%s%d -->", revisionMarkerPrefix, revision)
	if start := strings.Index(rendered, revisionMarkerPrefix); start >= 0 {
		if end := strings.Index(rendered[start:], "-->"); end >= 0 {
			end = start + end + len("-->")
			return rendered[:start] + marker + rendered[end:]
		}
	}
	if start := strings.Index(rendered, managedStart); start >= 0 {
		return rendered[:start] + marker + "\n" + rendered[start:]
	}
	if rendered == "" {
		return marker + "\n"
	}
	return rendered + "\n" + marker + "\n"
}

func goalFingerprint(goal string) string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return goalUnboundMarker
	}
	hash := sha256.Sum256([]byte(goal))
	return hex.EncodeToString(hash[:])
}

func parseGoalBinding(content string) (string, bool, error) {
	count := strings.Count(content, goalMarkerPrefix)
	if count == 0 {
		return "", false, nil
	}
	if count != 1 {
		return "", false, fmt.Errorf("invalid proton todo goal marker count")
	}
	start := strings.Index(content, goalMarkerPrefix)
	valueStart := start + len(goalMarkerPrefix)
	end := strings.Index(content[valueStart:], " -->")
	if end < 0 {
		return "", false, fmt.Errorf("invalid proton todo goal marker")
	}
	value := strings.TrimSpace(content[valueStart : valueStart+end])
	if value == goalUnboundMarker {
		return value, true, nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return "", false, fmt.Errorf("invalid proton todo goal fingerprint %q", value)
	}
	return value, true, nil
}

func renderGoalBinding(content, fingerprint string) string {
	marker := goalMarkerPrefix + fingerprint + " -->"
	if start := strings.Index(content, goalMarkerPrefix); start >= 0 {
		if end := strings.Index(content[start:], "-->"); end >= 0 {
			end = start + end + len("-->")
			return content[:start] + marker + content[end:]
		}
	}
	if start := strings.Index(content, managedStart); start >= 0 {
		return content[:start] + marker + "\n" + content[start:]
	}
	if content == "" {
		return marker + "\n"
	}
	sep := "\n"
	if strings.HasSuffix(content, "\n") {
		sep = ""
	}
	return content + sep + marker + "\n"
}
