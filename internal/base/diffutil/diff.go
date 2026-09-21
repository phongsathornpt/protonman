package diffutil

import (
	"crypto/sha1"
	"encoding/hex"
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

// GitBlobHash computes git's standard 7-character abbreviated object hash for content.
// When content is empty (representing a non-existent file in new or deleted states), it returns "0000000".
func GitBlobHash(content string) string {
	if content == "" {
		return "0000000"
	}
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))[:7]
}

// UnifiedDiff computes a git-compatible unified diff between original and modified text.
// If both texts are identical, it returns an empty string.
func UnifiedDiff(original, modified, filename string, contextLines int) string {
	return GitDiffFile(filename, filename, original, modified, contextLines)
}

// GitDiff is an alias for UnifiedDiff, computing a git-compatible unified diff.
func GitDiff(original, modified, filename string, contextLines int) string {
	return UnifiedDiff(original, modified, filename, contextLines)
}

// GitDiffFile computes a git-compatible unified diff between original and modified text,
// supporting file creation, modification, deletion, and renames.
func GitDiffFile(fromPath, toPath, original, modified string, contextLines int) string {
	cleanFrom := cleanDiffPath(fromPath)
	cleanTo := cleanDiffPath(toPath)

	if cleanFrom == cleanTo && original == modified {
		return ""
	}

	if len(original) > MaxDiffInputBytes || len(modified) > MaxDiffInputBytes {
		return formatOversizedNotice(cleanTo, len(original), len(modified))
	}
	if contextLines < 0 {
		contextLines = DefaultContextLines
	}

	// Pure rename with identical content
	if cleanFrom != cleanTo && original == modified {
		var header strings.Builder
		header.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", cleanFrom, cleanTo))
		header.WriteString("similarity index 100%\n")
		header.WriteString(fmt.Sprintf("rename from %s\n", cleanFrom))
		header.WriteString(fmt.Sprintf("rename to %s", cleanTo))
		return header.String()
	}

	origLines := difflib.SplitLines(original)
	modLines := difflib.SplitLines(modified)

	if len(origLines) > MaxDiffLines || len(modLines) > MaxDiffLines {
		return formatOversizedNotice(cleanTo, len(original), len(modified))
	}

	fromFile := "a/" + cleanFrom
	toFile := "b/" + cleanTo
	if original == "" {
		fromFile = "/dev/null"
	}
	if modified == "" {
		toFile = "/dev/null"
	}

	diff := difflib.UnifiedDiff{
		A:        origLines,
		B:        modLines,
		FromFile: fromFile,
		ToFile:   toFile,
		Context:  contextLines,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return ""
	}
	body := strings.TrimSpace(result)
	if body == "" && cleanFrom == cleanTo {
		return ""
	}

	var header strings.Builder
	header.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", cleanFrom, cleanTo))
	oldHash := GitBlobHash(original)
	newHash := GitBlobHash(modified)

	if original == "" {
		header.WriteString("new file mode 100644\n")
		header.WriteString(fmt.Sprintf("index 0000000..%s\n", newHash))
	} else if modified == "" {
		header.WriteString("deleted file mode 100644\n")
		header.WriteString(fmt.Sprintf("index %s..0000000\n", oldHash))
	} else if cleanFrom != cleanTo {
		header.WriteString(fmt.Sprintf("rename from %s\n", cleanFrom))
		header.WriteString(fmt.Sprintf("rename to %s\n", cleanTo))
		header.WriteString(fmt.Sprintf("index %s..%s 100644\n", oldHash, newHash))
	} else {
		header.WriteString(fmt.Sprintf("index %s..%s 100644\n", oldHash, newHash))
	}

	if body != "" {
		return header.String() + body
	}
	return strings.TrimSuffix(header.String(), "\n")
}

func cleanDiffPath(path string) string {
	clean := strings.TrimSpace(path)
	clean = strings.ReplaceAll(clean, "\\", "/")
	clean = strings.TrimPrefix(clean, "a/")
	clean = strings.TrimPrefix(clean, "b/")
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return "file"
	}
	return clean
}

func formatOversizedNotice(filename string, origLen, modLen int) string {
	return fmt.Sprintf("diff oversized for %s (%d bytes -> %d bytes, exceeds limit)", filename, origLen, modLen)
}

// DiffStats counts additions and deletions in a unified diff text.
// Header lines (+++, ---, diff, index, @@, mode, rename) are excluded from the counts.
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

	// Skip leading git/diff headers so the preview starts right at the hunk header
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
