//go:build desktop || desktop_gio

package conversation

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gioui.org/widget"
	"gioui.org/x/richtext"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type CacheKey struct {
	SessionID string
	ItemID    string
	Kind      desktopstate.TimelineKind
}

func MakeCacheKey(sessionID string, index int, item desktopstate.TimelineItem) CacheKey {
	itemID := item.ID
	if itemID == "" {
		itemID = "#" + fmt.Sprint(index)
	}
	return CacheKey{
		SessionID: sessionID,
		ItemID:    itemID,
		Kind:      item.Kind,
	}
}

type DisclosureButtons struct {
	Show     widget.Clickable
	Previous widget.Clickable
	Next     widget.Clickable
	Collapse widget.Clickable
}

type DescriptionCacheEntry struct {
	Source      desktopstate.TimelineItem
	Description string
}

type MarkdownCacheEntry struct {
	Source string
	Spans  []richtext.SpanStyle
	Bytes  int
	Plain  bool
}

type CodeCacheEntry struct {
	Source string
	Lang   string
	Spans  []richtext.SpanStyle
	Bytes  int
}

type ResponseCacheEntry struct {
	Source string
	Blocks []MarkdownBlock
}

func ItemDescription(item desktopstate.TimelineItem) string {
	var parts [4]string
	partCount := 0
	parts[partCount] = string(item.Kind)
	partCount++
	if item.Title != "" {
		parts[partCount] = item.Title
		partCount++
	}
	if item.Status != "" {
		parts[partCount] = item.Status
		partCount++
	}
	if item.Text != "" {
		parts[partCount] = item.Text
		partCount++
	}
	const maxDescriptionRunes = 512
	var description strings.Builder
	capacity := max(0, partCount-1) * 2
	for _, part := range parts[:partCount] {
		capacity += len(part)
	}
	description.Grow(min(capacity, maxDescriptionRunes*4))
	runeCount := 0
	prefixEnd := 0
	appendText := func(value string) bool {
		for index := 0; index < len(value); {
			if runeCount == maxDescriptionRunes {
				return false
			}
			_, width := utf8.DecodeRuneInString(value[index:])
			description.WriteString(value[index : index+width])
			index += width
			runeCount++
			if runeCount == maxDescriptionRunes-1 {
				prefixEnd = description.Len()
			}
		}
		return true
	}
	for index, part := range parts[:partCount] {
		if index > 0 && !appendText(", ") {
			return strings.TrimSpace(description.String()[:prefixEnd]) + "…"
		}
		if !appendText(part) {
			return strings.TrimSpace(description.String()[:prefixEnd]) + "…"
		}
	}
	return strings.TrimSpace(description.String())
}
