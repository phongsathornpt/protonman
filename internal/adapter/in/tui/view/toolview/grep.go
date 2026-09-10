package toolview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

// FormatGrepView formats grep lines with syntax styling, keyword highlighting,
// and horizontal width clamping to prevent wrapping explosion in the viewport.
func FormatGrepView(lines []string, target string, width int) []string {
	if len(lines) == 0 {
		return nil
	}

	terms := extractGrepQueryTerms(target)
	formatted := make([]string, 0, len(lines))
	contentWidth := max(20, width-6)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			formatted = append(formatted, tuistyle.ToolFoldStyle.Render(trimmed))
			continue
		}
		parts := strings.SplitN(trimmed, ":", 3)
		if len(parts) == 3 {
			file := parts[0]
			lineNum := parts[1]
			content := parts[2]

			avail := contentWidth - ansi.StringWidth(file) - len(lineNum) - 4
			cleanContent := strings.TrimSpace(content)
			if avail > 10 && ansi.StringWidth(cleanContent) > avail {
				cleanContent = textview.TruncateEllipsis(cleanContent, avail)
			}

			highlightedContent := highlightGrepTerms(cleanContent, terms)
			lineStr := tuistyle.ToolTargetStyle.Render(file) + tuistyle.MutedStyle.Render(":"+lineNum+": ") + highlightedContent
			formatted = append(formatted, lineStr)
		} else {
			if ansi.StringWidth(trimmed) > contentWidth {
				trimmed = textview.TruncateEllipsis(trimmed, contentWidth)
			}
			formatted = append(formatted, tuistyle.BodyStyle.Render(trimmed))
		}
	}

	return FormatOutputFold(formatted, 4)
}

func extractGrepQueryTerms(target string) []string {
	clean := strings.TrimSpace(target)
	if strings.Contains(clean, " in ") {
		clean = strings.SplitN(clean, " in ", 2)[0]
	}
	clean = strings.Trim(clean, `"`)
	if clean == "" {
		return nil
	}
	rawTerms := strings.Split(clean, "|")
	terms := make([]string, 0, len(rawTerms))
	for _, t := range rawTerms {
		t = strings.TrimSpace(t)
		if len(t) >= 2 {
			terms = append(terms, t)
		}
	}
	return terms
}

func highlightGrepTerms(content string, terms []string) string {
	if len(terms) == 0 || content == "" {
		return tuistyle.BodyStyle.Render(content)
	}
	result := content
	for _, term := range terms {
		idx := strings.Index(strings.ToLower(result), strings.ToLower(term))
		if idx != -1 && idx+len(term) <= len(result) {
			matched := result[idx : idx+len(term)]
			return tuistyle.BodyStyle.Render(result[:idx]) + tuistyle.BrandStyle.Render(matched) + tuistyle.BodyStyle.Render(result[idx+len(term):])
		}
	}
	return tuistyle.BodyStyle.Render(result)
}
