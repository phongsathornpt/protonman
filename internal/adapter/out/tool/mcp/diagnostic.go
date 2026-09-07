package mcp

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func diagnosticFirstLine(value string, width int) string {
	value = strings.TrimSpace(ansi.Strip(value))
	if value == "" {
		return ""
	}
	if idx := strings.IndexAny(value, "\r\n"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	if width <= 0 || ansi.StringWidth(value) <= width {
		return value
	}
	return ansi.Truncate(value, width, "...")
}
