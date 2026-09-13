package memory

import "regexp"

const redactedSecret = "[REDACTED_SECRET]"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(api[_ -]?key|access[_ -]?token|refresh[_ -]?token|authorization|password|passwd|secret)\b\s*[:=]\s*[^\s,;]+`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+\-/]+=*`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
}

func redactSecrets(value string) string {
	for index, pattern := range secretPatterns {
		if index == 0 {
			value = pattern.ReplaceAllStringFunc(value, func(match string) string {
				parts := regexp.MustCompile(`\s*[:=]\s*`).Split(match, 2)
				if len(parts) == 0 {
					return redactedSecret
				}
				return parts[0] + "=" + redactedSecret
			})
			continue
		}
		value = pattern.ReplaceAllString(value, redactedSecret)
	}
	return value
}
