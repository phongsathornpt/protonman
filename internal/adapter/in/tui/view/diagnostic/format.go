package diagnostic

import "fmt"

// FormatSummary produces a concise one-line summary using Protonman's stable
// provider-independent error code. Upstream status/badge data remains attached
// to Error for raw diagnostics only.
func FormatSummary(c Error) string {
	if c.Kind != "" && c.Kind != KindGeneric {
		return fmt.Sprintf("[%s] %s: %s", UserCode(c.Kind), c.Title, c.Message)
	}
	return fmt.Sprintf("%s: %s", c.Title, c.Message)
}
