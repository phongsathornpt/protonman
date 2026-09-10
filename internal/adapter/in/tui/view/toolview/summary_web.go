package toolview

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

var titleRegex = regexp.MustCompile(`(?i)<title[^>]*>([\s\S]*?)</title>`)

func summarizeWebFetch(body string, truncated bool) string {
	if body == "" {
		return "0 B"
	}
	if strings.HasPrefix(body, "[binary content omitted") {
		return body
	}

	sizeStr := formatByteSize(len(body))
	if truncated {
		sizeStr += "+ truncated"
	}

	// Try extracting HTML title
	if matches := titleRegex.FindStringSubmatch(body); len(matches) > 1 {
		rawTitle := html.UnescapeString(strings.TrimSpace(matches[1]))
		rawTitle = strings.Join(strings.Fields(rawTitle), " ")
		if rawTitle != "" {
			return fmt.Sprintf("%q (%s)", textview.TruncateEllipsis(rawTitle, 45), sizeStr)
		}
	}

	// Check if JSON
	if (strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}")) ||
		(strings.HasPrefix(body, "[") && strings.HasSuffix(body, "]")) {
		var anyVal any
		if json.Unmarshal([]byte(body), &anyVal) == nil {
			switch val := anyVal.(type) {
			case []any:
				return fmt.Sprintf("JSON array (%d items, %s)", len(val), sizeStr)
			case map[string]any:
				return fmt.Sprintf("JSON object (%d keys, %s)", len(val), sizeStr)
			}
		}
	}

	lines := strings.Count(body, "\n") + 1
	return fmt.Sprintf("%d lines (%s)", lines, sizeStr)
}

func summarizeWebSearch(body string) string {
	if body == "" {
		return "0 results"
	}
	lines := strings.Split(body, "\n")
	nonEmpty := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
	}
	if nonEmpty == 1 {
		return "1 result found"
	}
	return fmt.Sprintf("%d results found", nonEmpty)
}
