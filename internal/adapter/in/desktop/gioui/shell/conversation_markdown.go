//go:build desktop || desktop_gio

package shell

import (
	"fmt"
	"image/color"
	"io"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/x/richtext"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

// Markdown and code rendering, including the span caches that keep re-parsing
// a rendered block off the layout goroutine.
func (s *Shell) layoutMarkdown(gtx layout.Context, key conversationCacheKey, source string, fallback color.NRGBA) layout.Dimensions {
	if strings.TrimSpace(source) == "" {
		s.dropMarkdownCache(key)
		return layout.Dimensions{}
	}
	if len(source) > maxMarkdownRenderBytes {
		s.dropMarkdownCache(key)
		return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
	}
	cached, ok := s.conversationCache[key]
	if !ok || cached.source != source {
		spans, err := s.conversationMarkdown.Render([]byte(source))
		if err != nil {
			s.dropMarkdownCache(key)
			cached = conversationMarkdownCache{source: source, plain: true}
			if !s.storeMarkdownCache(key, cached) {
				return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
			}
			cached = s.conversationCache[key]
		} else {
			for index := range spans {
				spans[index].Interactive = false
			}
			cached = conversationMarkdownCache{source: source, spans: spans}
			if !s.storeMarkdownCache(key, cached) {
				return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
			}
			cached = s.conversationCache[key]
		}
	}
	if cached.plain {
		return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
	}
	return richtext.Text(nil, s.theme.Material.Shaper, cached.spans...).Layout(gtx)
}

func (s *Shell) responseBlocks(key conversationCacheKey, source string) []markdownBlock {
	if cached, ok := s.conversationResponseCache[key]; ok && cached.source == source {
		return cached.blocks
	}
	blocks := splitMarkdownCodeBlocks(source)
	if source == "" || len(source) > maxResponseSplitSourceBytes {
		return blocks
	}
	if s.conversationResponseCache == nil {
		s.conversationResponseCache = make(map[conversationCacheKey]conversationResponseCache)
	}
	entryBytes := responseSplitRetainedEstimate + len(source) + len(blocks)*16
	if entryBytes > maxResponseSplitCacheBytes {
		return blocks
	}
	if previous, ok := s.conversationResponseCache[key]; ok {
		s.conversationResponseBytes -= responseSplitRetainedEstimate + len(previous.source) + len(previous.blocks)*16
		delete(s.conversationResponseCache, key)
		for index, id := range s.conversationResponseOrder {
			if id == key {
				s.conversationResponseOrder = append(s.conversationResponseOrder[:index], s.conversationResponseOrder[index+1:]...)
				break
			}
		}
	}
	for len(s.conversationResponseCache) >= maxResponseSplitCacheEntries || s.conversationResponseBytes+entryBytes > maxResponseSplitCacheBytes {
		if len(s.conversationResponseOrder) == 0 {
			break
		}
		oldest := s.conversationResponseOrder[0]
		s.conversationResponseOrder = s.conversationResponseOrder[1:]
		if previous, ok := s.conversationResponseCache[oldest]; ok {
			delete(s.conversationResponseCache, oldest)
			s.conversationResponseBytes -= responseSplitRetainedEstimate + len(previous.source) + len(previous.blocks)*16
		}
	}
	s.conversationResponseCache[key] = conversationResponseCache{source: source, blocks: blocks}
	s.conversationResponseOrder = append(s.conversationResponseOrder, key)
	s.conversationResponseBytes += entryBytes
	return blocks
}

func (s *Shell) layoutRichResponse(gtx layout.Context, key conversationCacheKey, blocks []markdownBlock, foreground color.NRGBA) layout.Dimensions {
	if len(blocks) == 0 {
		return layout.Dimensions{}
	}
	if len(blocks) == 1 && blocks[0].Kind == markdownBlockText {
		return s.layoutMarkdown(gtx, key, blocks[0].Text, foreground)
	}

	blockChildren := make([]layout.FlexChild, 0, len(blocks))
	for idx, b := range blocks {
		blockIdx := idx
		block := b
		subKey := conversationCacheKey{
			sessionID: key.sessionID,
			itemID:    fmt.Sprintf("%s-b%d", key.itemID, blockIdx),
		}
		if block.Kind == markdownBlockCode {
			blockChildren = append(blockChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 6, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutCodeCard(gtx, subKey, block.Lang, block.Code)
				})
			}))
		} else {
			if strings.TrimSpace(block.Text) != "" {
				blockChildren = append(blockChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMarkdown(gtx, subKey, block.Text, foreground)
				}))
			}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, blockChildren...)
}

func (s *Shell) layoutCodeCard(gtx layout.Context, key conversationCacheKey, lang, code string) layout.Dimensions {
	keyStr := key.sessionID + ":" + key.itemID
	copyBtn, ok := s.codeCopyButtons[keyStr]
	if !ok {
		copyBtn = new(widget.Clickable)
		if s.codeCopyButtons == nil {
			s.codeCopyButtons = make(map[string]*widget.Clickable)
		}
		s.codeCopyButtons[keyStr] = copyBtn
	}

	if copyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(code)),
		})
		if s.codeCopiedAt == nil {
			s.codeCopiedAt = make(map[string]time.Time)
		}
		s.codeCopiedAt[keyStr] = time.Now()
	}

	copied := false
	if s.codeCopiedAt != nil {
		if t, ok := s.codeCopiedAt[keyStr]; ok && time.Since(t) < 2*time.Second {
			copied = true
		}
	}

	displayLang := strings.ToUpper(strings.TrimSpace(lang))
	if displayLang == "" {
		displayLang = "CODE"
	}

	return s.roundedBorderSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLowest, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 4, Bottom: 4, Left: 10, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, displayLang, textLabelSmall, font.Bold, s.theme.Colors.OnSurfaceVariant, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btnText := "Copy"
								btnColor := s.theme.Colors.OnSurfaceVariant
								if copied {
									btnText = "✓ Copied"
									btnColor = s.theme.Colors.OnSuccessContainer
								}
								return copyBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, btnText, textLabelSmall, font.Medium, btnColor, 1)
									})
								})
							}),
						)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					spans := s.cachedCodeSpans(key, lang, code)
					return richtext.Text(nil, s.theme.Material.Shaper, spans...).Layout(gtx)
				})
			}),
		)
	})
}

func codeCacheEntryBytes(key conversationCacheKey, cached conversationCodeCache) int {
	bytes := 128 + len(key.sessionID) + len(key.itemID) + len(cached.lang) + len(cached.source)
	for _, span := range cached.spans {
		bytes += markdownSpanRetainedEstimate + len(span.Content)
	}
	return bytes
}

func (s *Shell) cachedCodeSpans(key conversationCacheKey, lang, code string) []richtext.SpanStyle {
	if cached, ok := s.conversationCodeCache[key]; ok && cached.source == code && cached.lang == lang {
		return cached.spans
	}
	spans := highlightCodeSpans(s.theme, lang, code)
	entry := conversationCodeCache{source: code, lang: lang, spans: spans}
	entry.bytes = codeCacheEntryBytes(key, entry)
	if entry.bytes > maxConversationCacheBytes {
		return spans
	}
	if s.conversationCodeCache == nil {
		s.conversationCodeCache = make(map[conversationCacheKey]conversationCodeCache)
	}
	if _, ok := s.conversationCodeCache[key]; ok {
		for index, id := range s.conversationCodeCacheOrder {
			if id == key {
				s.conversationCodeCacheOrder = append(s.conversationCodeCacheOrder[:index], s.conversationCodeCacheOrder[index+1:]...)
				break
			}
		}
		if previous, ok := s.conversationCodeCache[key]; ok {
			s.conversationCodeCacheBytes -= previous.bytes
		}
		delete(s.conversationCodeCache, key)
	}
	for len(s.conversationCodeCache) >= maxConversationCacheEntries || s.conversationCodeCacheBytes+entry.bytes > maxConversationCacheBytes {
		if len(s.conversationCodeCacheOrder) == 0 {
			break
		}
		oldest := s.conversationCodeCacheOrder[0]
		s.conversationCodeCacheOrder = s.conversationCodeCacheOrder[1:]
		if previous, ok := s.conversationCodeCache[oldest]; ok {
			delete(s.conversationCodeCache, oldest)
			s.conversationCodeCacheBytes -= previous.bytes
		}
	}
	s.conversationCodeCache[key] = entry
	s.conversationCodeCacheOrder = append(s.conversationCodeCacheOrder, key)
	s.conversationCodeCacheBytes += entry.bytes
	return spans
}

func markdownCacheEntryBytes(key conversationCacheKey, cached conversationMarkdownCache) int {
	bytes := 128 + len(key.sessionID) + len(key.itemID) + len(cached.source)
	for _, span := range cached.spans {
		bytes += markdownSpanRetainedEstimate + len(span.Content)
	}
	return bytes
}

func (s *Shell) storeMarkdownCache(key conversationCacheKey, cached conversationMarkdownCache) bool {
	cached.bytes = markdownCacheEntryBytes(key, cached)
	if cached.bytes > maxConversationCacheBytes {
		cached.spans = nil
		cached.plain = true
		cached.bytes = markdownCacheEntryBytes(key, cached)
		if cached.bytes > maxConversationCacheBytes {
			s.dropMarkdownCache(key)
			return false
		}
	}
	if previous, ok := s.conversationCache[key]; ok {
		s.conversationCacheBytes -= previous.bytes
		delete(s.conversationCache, key)
		for index, id := range s.conversationCacheOrder {
			if id == key {
				s.conversationCacheOrder = append(s.conversationCacheOrder[:index], s.conversationCacheOrder[index+1:]...)
				break
			}
		}
	}
	for len(s.conversationCache) >= maxConversationCacheEntries || s.conversationCacheBytes+cached.bytes > maxConversationCacheBytes {
		if len(s.conversationCacheOrder) == 0 {
			break
		}
		oldest := s.conversationCacheOrder[0]
		s.conversationCacheOrder = s.conversationCacheOrder[1:]
		if previous, ok := s.conversationCache[oldest]; ok {
			delete(s.conversationCache, oldest)
			s.conversationCacheBytes -= previous.bytes
		}
	}
	s.conversationCache[key] = cached
	s.conversationCacheOrder = append(s.conversationCacheOrder, key)
	s.conversationCacheBytes += cached.bytes
	return true
}

func (s *Shell) dropMarkdownCache(key conversationCacheKey) {
	if cached, ok := s.conversationCache[key]; ok {
		delete(s.conversationCache, key)
		s.conversationCacheBytes -= cached.bytes
	}
	for index, id := range s.conversationCacheOrder {
		if id == key {
			s.conversationCacheOrder = append(s.conversationCacheOrder[:index], s.conversationCacheOrder[index+1:]...)
			break
		}
	}
}
