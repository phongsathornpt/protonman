package memory

import "regexp"

const redactedSecret = "[REDACTED_SECRET]"

var (
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(["']?\b(api[_ -]?key|access[_ -]?token|refresh[_ -]?token|authorization|password|passwd|secret)\b["']?\s*[:=]\s*["']?)[^"'\s,;}]+["']?`),
		regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+\-/]+=*`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{20,}\b`),
		regexp.MustCompile(`\b(?:npm|pypi|glpat)-[A-Za-z0-9_-]{16,}\b`),
	}
)

func redactSecrets(value string) string {
	for index, pattern := range secretPatterns {
		if index == 0 {
			value = pattern.ReplaceAllString(value, "$1"+redactedSecret)
			continue
		}
		value = pattern.ReplaceAllString(value, redactedSecret)
	}
	return value
}
