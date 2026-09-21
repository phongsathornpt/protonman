package diffutil

import (
	"fmt"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

const (
	// DefaultContextLines is the standard unified diff context line count.
	DefaultContextLines = 3
	// MaxDiffInputBytes bounds the size of text sent to the diff algorithm to prevent latency spikes.
	MaxDiffInputBytes = 2 * 1024 * 1024 // 2 MB
	// MaxDiffLines bounds line counts to protect against quadratic diff comparisons.
	MaxDiffLines = 15000
)

// UnifiedDiff computes a standard unified diff between original and modified text.
// If both texts are identical, it returns an empty string.
func UnifiedDiff(original, modified, filename string, contextLines int) string {
	if original == modified {
		return ""
	}
	if len(original) > MaxDiffInputBytes || len(modified) > MaxDiffInputBytes {
		return formatOversizedNotice(filename, len(original), len(modified))
	}
	if contextLines < 0 {
		contextLines = DefaultContextLines
	}

	cleanPath := strings.TrimSpace(filename)
	if cleanPath == "" {
		cleanPath = "file"
	}

	origLines := difflib.SplitLines(original)
	modLines := difflib.SplitLines(modified)

	if len(origLines) > MaxDiffLines || len(modLines) > MaxDiffLines {
		return formatOversizedNotice(filename, len(original), len(modified))
	}

	diff := difflib.UnifiedDiff{
		A:        origLines,
		B:        modLines,
		FromFile: "a/" + cleanPath,
		ToFile:   "b/" + cleanPath,
		Context:  contextLines,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(result, "\n")
}

func formatOversizedNotice(filename string, origLen, modLen int) string {
	return fmt.Sprintf("diff oversized for %s (%d bytes -> %d bytes, exceeds limit)", filename, origLen, modLen)
}

// DiffStats counts additions and deletions in a unified diff text.
// Header lines (+++, ---, diff, index, @@) are excluded from the counts.
func DiffStats(diffText string) (additions, deletions int) {
	if strings.TrimSpace(diffText) == "" {
		return 0, 0
	}
	lines := strings.Split(diffText, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "+++") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		if strings.HasPrefix(trimmed, "+") {
			additions++
		} else if strings.HasPrefix(trimmed, "-") {
			deletions++
		}
	}
	return additions, deletions
}

// StatBadge formats additions and deletions into a concise badge like "+4 -2".
func StatBadge(additions, deletions int) string {
	if additions == 0 && deletions == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	if additions > 0 {
		parts = append(parts, fmt.Sprintf("+%d", additions))
	}
	if deletions > 0 {
		parts = append(parts, fmt.Sprintf("-%d", deletions))
	}
	return strings.Join(parts, " ")
}

// ExtractPreview extracts up to maxLines of diff content (starting from the first @@ hunk header)
// and returns the preview lines along with the count of remaining omitted diff lines.
func ExtractPreview(diffText string, maxLines int) (preview []string, remaining int) {
	if strings.TrimSpace(diffText) == "" {
		return nil, 0
	}
	allLines := strings.Split(diffText, "\n")

	// Skip leading "--- " or "+++ " lines so the preview starts right at the hunk header
	startIdx := 0
	for i, line := range allLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@@") {
			startIdx = i
			break
		}
	}

	contentLines := allLines[startIdx:]
	if maxLines <= 0 || len(contentLines) <= maxLines {
		return contentLines, 0
	}

	return contentLines[:maxLines], len(contentLines) - maxLines
}
