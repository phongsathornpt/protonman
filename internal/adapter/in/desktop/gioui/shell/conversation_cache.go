//go:build desktop || desktop_gio

package shell

import (
	"fmt"

	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/richtext"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Conversation render caches. Every cache here is identity-bounded rather than
// LRU: replacing an entry must not reorder eviction, because the entry usually
// still corresponds to a visible timeline item.
const (
	maxComposerDrafts                    = 32
	maxStreamingTextBytes                = 1 << 10
	maxConversationCacheEntries          = 512
	maxConversationCacheBytes            = 4 << 20
	maxConversationTextWidth     unit.Dp = 840
	maxConversationExpansionKeys         = 256
	// maxMarkdownRenderBytes bounds what gets shaped through the markdown
	// richtext path. Text shaping cost scales with total message length even
	// though only a viewport is visible, so anything larger renders through
	// the paged plain-text path instead; profiling showed a 100 KiB message
	// shaping every frame dominated mixed-prose frame time.
	maxMarkdownRenderBytes        = 16 << 10
	maxResponseSplitCacheEntries  = 128
	maxResponseSplitCacheBytes    = 2 << 20
	maxResponseSplitSourceBytes   = 256 << 10
	responseSplitRetainedEstimate = 96
	markdownSpanRetainedEstimate  = 192
	maxToolDiffCacheEntries       = 512
	maxThinkingParseCacheEntries  = 512
	// Caps the per-item accessibility description memo; conversation frames
	// rebuild visible descriptions every redraw, which dominated frame
	// allocations in tool-heavy sessions.
	maxConversationDescriptionCacheEntries = 512
	// Paged large-message geometry. Previews and pages stay small so a fully
	// expanded message shapes at most one bounded page per frame.
	maxMessagePreviewBytes   = 8 << 10
	messagePageBytes         = 8 << 10
	largeMessagePreviewLines = 48
	// Tool outputs above this threshold render through the paged large-message
	// pattern instead of laying out the full text every frame. Expanded tool
	// bodies (terminal dumps in particular) previously laid out unbounded text
	// on every scroll frame.
	maxToolOutputUnpagedBytes = 8 << 10
)

type descriptionCacheEntry struct {
	source      desktopstate.TimelineItem
	description string
}

type toolDiffCacheEntry struct {
	source    string
	isDiff    bool
	filename  string
	additions int
	deletions int
	preview   []string
	omitted   int
}

type thinkingParseCacheEntry struct {
	source string
	parsed parsedAssistantMessage
}

type conversationMarkdownCache struct {
	source string
	spans  []richtext.SpanStyle
	bytes  int
	plain  bool
}

type conversationCodeCache struct {
	source string
	lang   string
	spans  []richtext.SpanStyle
	bytes  int
}

type conversationResponseCache struct {
	source string
	blocks []markdownBlock
}

type conversationCacheKey struct {
	sessionID string
	itemID    string
	kind      desktopstate.TimelineKind
}

type conversationDisclosureButtons struct {
	show     widget.Clickable
	previous widget.Clickable
	next     widget.Clickable
	collapse widget.Clickable
}

func makeConversationCacheKey(sessionID string, index int, item desktopstate.TimelineItem) conversationCacheKey {
	itemID := item.ID
	if itemID == "" {
		itemID = "#" + fmt.Sprint(index)
	}
	return conversationCacheKey{
		sessionID: sessionID,
		itemID:    itemID,
		kind:      item.Kind,
	}
}

func conversationItemDescription(item desktopstate.TimelineItem) string {
	return conversationcomponent.ItemDescription(item)
}
