//go:build desktop || desktop_gio

package conversation

import (
	"strings"
	"unicode/utf8"

	"gioui.org/widget"
)

const (
	MaxMessagePreviewBytes    = 8 << 10
	MessagePageBytes          = 8 << 10
	MaxStreamingTextBytes     = 1 << 10
	MaxToolOutputUnpagedBytes = 8 << 10
)

type ToolOutputPageButtons struct {
	Previous widget.Clickable
	Next     widget.Clickable
	Collapse widget.Clickable
}

// ToolOutputWindow returns the visible page text and paging facts for a tool output.
func ToolOutputWindow(source string, page int) (text string, pageCount, current int) {
	pageCount = (len(source) + MessagePageBytes - 1) / MessagePageBytes
	if page < 0 {
		page = 0
	} else if page >= pageCount {
		page = pageCount - 1
	}
	start := page * MessagePageBytes
	for start > 0 && start < len(source) && !utf8.RuneStart(source[start]) {
		start--
	}
	end := min((page+1)*MessagePageBytes, len(source))
	for end > start && end < len(source) && !utf8.RuneStart(source[end]) {
		end--
	}
	return source[start:end], pageCount, page
}

// LargeMessageWindow returns the windowed text and byte offsets for a paged message.
func LargeMessageWindow(source string, page int) (string, int, int) {
	pageCount := (len(source) + MessagePageBytes - 1) / MessagePageBytes
	if page < 0 {
		page = 0
	} else if page >= pageCount {
		page = pageCount - 1
	}
	start := page * MessagePageBytes
	for start > 0 && start < len(source) && !utf8.RuneStart(source[start]) {
		start--
	}
	end := min((page+1)*MessagePageBytes, len(source))
	for end > start && end < len(source) && !utf8.RuneStart(source[end]) {
		end--
	}
	return strings.Clone(source[start:end]), start, end
}

// LargeMessagePreview returns the truncated head of a large message for unexpanded view.
func LargeMessagePreview(source string) string {
	if len(source) <= MaxMessagePreviewBytes {
		return source
	}
	end := MaxMessagePreviewBytes
	for end > 0 && !utf8.RuneStart(source[end]) {
		end--
	}
	return source[:end]
}

// StreamingText returns the trailing window of an in-progress streaming message.
func StreamingText(source string) string {
	if len(source) <= MaxStreamingTextBytes {
		return source
	}
	start := len(source) - MaxStreamingTextBytes
	for start < len(source) && !utf8.RuneStart(source[start]) {
		start++
	}
	return "…\n" + source[start:]
}
