package tui

import "strings"

// UserCell renders submitted user input.
type UserCell struct{ Text string }

func (UserCell) Kind() HistoryCellKind { return HistoryCellUser }
func (c UserCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c UserCell) RenderWidth(width int) []string {
	lines := safeWrappedLines(strings.TrimRight(c.Text, "\n"), maxInt(1, width-2))
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for index, line := range lines {
		prefix := "  "
		if index == 0 {
			prefix = glyphMark
		}
		out = append(out, userStyle.Render(prefix)+bodyStyle.Render(line))
	}
	return out
}
func (c UserCell) RawLines() []string { return rawTextLines(c.Text) }
func (c UserCell) LineCount() int     { return len(c.RawLines()) }

// AssistantCell is mutable while assistant output is streaming.
type AssistantCell struct {
	Text          string
	renderCache   assistantRenderCache
	streamBuilder strings.Builder
	streamText    string
	streamActive  bool
}

type assistantRenderCache struct {
	width       int
	processed   int
	processedAt string
	lines       []string
	decorated   []string
	state       markdownRenderState
}

func (c *AssistantCell) appendDelta(delta string) {
	if delta == "" {
		return
	}
	if !c.streamActive || c.Text != c.streamText {
		c.streamBuilder.Reset()
		c.streamBuilder.Grow(len(c.Text) + len(delta))
		c.streamBuilder.WriteString(c.Text)
		c.streamActive = true
	}
	c.streamBuilder.WriteString(delta)
	c.Text = c.streamBuilder.String()
	c.streamText = c.Text
}

func (c *AssistantCell) sealStream() {
	if !c.streamActive {
		return
	}
	c.Text = strings.Clone(c.Text)
	c.streamBuilder.Reset()
	c.streamText = ""
	c.streamActive = false
	if c.renderCache.processed > 0 && c.renderCache.processed <= len(c.Text) {
		c.renderCache.processedAt = assistantCacheTail(c.Text[:c.renderCache.processed])
	}
}

func (*AssistantCell) Kind() HistoryCellKind { return HistoryCellAssistant }
func (c *AssistantCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c *AssistantCell) RenderWidth(width int) []string {
	text := assistantIncrementalText(c.Text)
	if text == "" {
		return nil
	}
	return c.renderAssistantIncremental(text, maxInt(8, width-2))
}

func assistantIncrementalText(text string) string {
	if !strings.HasSuffix(text, "\n") {
		return text
	}
	end := len(text) - 1
	for end > 0 && text[end-1] == '\n' {
		end--
	}
	return text[:end+1]
}

func (c *AssistantCell) renderAssistantIncremental(text string, width int) []string {
	if strings.ContainsRune(text, '\r') {
		c.renderCache = assistantRenderCache{}
		return decorateAssistantLines(renderMarkdownLines(text, width), 0)
	}
	cache, completeEnd := c.updateAssistantRenderCache(text, width)
	stableLen := len(cache.decorated)
	state := cache.state
	tail := text[completeEnd:]
	if tail == "" && !state.InFence() {
		for stableLen > 0 && cache.lines[stableLen-1] == "" {
			stableLen--
		}
	}
	if tail == "" && !state.InFence() {
		return cache.decorated[:stableLen]
	}
	out := append([]string(nil), cache.decorated[:stableLen]...)
	if tail != "" {
		tailLines := renderMarkdownLine(tail, width, &state)
		out = append(out, decorateAssistantLines(tailLines, stableLen)...)
	}
	if state.InFence() {
		marker := markdownCodeStyle.Render("  └─ code (unterminated)")
		out = append(out, assistantDecoratedLine(marker, len(out)))
	}
	return out
}

func (c *AssistantCell) renderMarkdownIncremental(text string, width int) []string {
	if strings.ContainsRune(text, '\r') {
		c.renderCache = assistantRenderCache{}
		return renderMarkdownLines(text, width)
	}
	cache, completeEnd := c.updateAssistantRenderCache(text, width)
	out := append([]string(nil), cache.lines...)
	state := cache.state
	if tail := text[completeEnd:]; tail != "" {
		out = append(out, renderMarkdownLine(tail, width, &state)...)
	}
	if state.InFence() {
		out = append(out, markdownCodeStyle.Render("  └─ code (unterminated)"))
	}
	return trimTrailingBlankLines(out)
}

func (c *AssistantCell) updateAssistantRenderCache(text string, width int) (*assistantRenderCache, int) {
	cache := &c.renderCache
	if cache.width != width || cache.processed > len(text) || !assistantCachePrefixMatches(text, cache) {
		*cache = assistantRenderCache{width: width}
	}
	completeEnd := strings.LastIndexByte(text, '\n') + 1
	if completeEnd < cache.processed {
		*cache = assistantRenderCache{width: width}
	}
	if cache.processed < completeEnd {
		segment := text[cache.processed:completeEnd]
		for offset := 0; offset < len(segment); {
			relativeEnd := strings.IndexByte(segment[offset:], '\n')
			if relativeEnd < 0 {
				break
			}
			end := offset + relativeEnd
			lines := renderMarkdownLine(segment[offset:end], width, &cache.state)
			start := len(cache.lines)
			cache.lines = append(cache.lines, lines...)
			cache.decorated = appendAssistantDecoratedLines(cache.decorated, lines, start)
			offset = end + 1
		}
		cache.processed = completeEnd
		cache.processedAt = assistantCacheTail(text[:completeEnd])
	}
	return cache, completeEnd
}

func appendAssistantDecoratedLines(dst []string, lines []string, start int) []string {
	for index, line := range lines {
		dst = append(dst, assistantDecoratedLine(line, start+index))
	}
	return dst
}

func decorateAssistantLines(lines []string, start int) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	for index, line := range lines {
		out[index] = assistantDecoratedLine(line, start+index)
	}
	return out
}

func assistantDecoratedLine(line string, index int) string {
	if index == 0 {
		return "● " + line
	}
	return "  " + line
}

func assistantCachePrefixMatches(text string, cache *assistantRenderCache) bool {
	if cache.processed == 0 || cache.processedAt == "" {
		return true
	}
	if cache.processed > len(text) || len(cache.processedAt) > cache.processed {
		return false
	}
	start := cache.processed - len(cache.processedAt)
	return text[start:cache.processed] == cache.processedAt
}

func assistantCacheTail(text string) string {
	const tailBytes = 64
	if len(text) <= tailBytes {
		return text
	}
	return text[len(text)-tailBytes:]
}

func (c *AssistantCell) RawLines() []string { return rawTextLines(c.Text) }
func (c *AssistantCell) LineCount() int     { return len(c.RawLines()) }
