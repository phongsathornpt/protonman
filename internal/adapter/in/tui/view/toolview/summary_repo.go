package toolview

import (
	"fmt"
	"strings"
)

func summarizeGrep(body string, truncated bool) string {
	if body == "" {
		return "no matches found"
	}
	lines := strings.Split(body, "\n")
	matchCount := 0
	files := make(map[string]struct{})
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		matchCount++
		if idx := strings.Index(trimmed, ":"); idx != -1 {
			files[trimmed[:idx]] = struct{}{}
		}
	}
	if matchCount == 0 {
		return "no matches found"
	}
	fileCount := len(files)
	matchWord := "matches"
	if matchCount == 1 {
		matchWord = "match"
	}
	fileWord := "files"
	if fileCount == 1 {
		fileWord = "file"
	}
	suffix := ""
	if truncated {
		suffix = " (truncated)"
	}
	return fmt.Sprintf("%d %s across %d %s%s", matchCount, matchWord, fileCount, fileWord, suffix)
}

func summarizeGitStatus(body string) string {
	if body == "" || strings.Contains(body, "nothing to commit") || strings.Contains(body, "working tree clean") {
		return "working tree clean"
	}
	lines := strings.Split(body, "\n")
	modified, untracked, staged := 0, 0, 0
	for _, l := range lines {
		if len(l) < 3 {
			continue
		}
		x := l[0]
		y := l[1]
		if x == '?' && y == '?' {
			untracked++
			continue
		}
		if x == 'M' || x == 'A' || x == 'D' || x == 'R' || x == 'C' {
			staged++
		}
		if y == 'M' || y == 'D' {
			modified++
		}
	}
	parts := make([]string, 0, 3)
	if staged > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", staged))
	}
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", modified))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}
	if len(parts) == 0 {
		return "status updated"
	}
	return strings.Join(parts, ", ")
}

func summarizeEdit(_ string, body string) string {
	if body == "" {
		return "updated"
	}
	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, "restored checkpoint"):
		return "restored checkpoint"
	case strings.Contains(lower, "wrote file successfully"):
		return "saved"
	case strings.Contains(lower, "success. updated"), strings.Contains(lower, "success. added"), strings.Contains(lower, "success. deleted"):
		return "patch applied"
	case strings.Contains(lower, "has been updated"), strings.Contains(lower, "has been created"):
		return "1 replacement applied"
	default:
		return "file updated"
	}
}
