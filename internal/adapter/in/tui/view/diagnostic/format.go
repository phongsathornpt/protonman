package diagnostic

import "fmt"

// FormatSummary produces a concise one-line summary of a classified error.
func FormatSummary(c Error) string {
	if c.Badge != "" {
		return fmt.Sprintf("[%s] %s: %s", c.Badge, c.Title, c.Message)
	}
	return fmt.Sprintf("%s: %s", c.Title, c.Message)
}
