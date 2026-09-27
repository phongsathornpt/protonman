package history

import "strings"

func (s *HistoryState) ReleaseAlternateRenderCache() {
	if s == nil {
		return
	}
	s.invalidateAlternateRenderCache()
}

// ReleaseRawTextCache drops the full raw transcript materialization. Raw text is
// only needed while the transcript overlay is in raw mode and should not retain
// a duplicate transcript after the overlay is closed or switched back to rich.
func (s *HistoryState) ReleaseRawTextCache() {
	if s == nil {
		return
	}
	s.cachedRawText = ""
	s.rawTextValid = false
}

func (s *HistoryState) invalidateAlternateRenderCache() {
	s.altRender = nil
	s.altRenderValid = false
	s.altRenderWidth = 0
}

func releaseCommittedCellRenderCache(cell HistoryCell) {
	if assistant, ok := cell.(*AssistantCell); ok {
		assistant.releaseRenderCache()
	}
}

func (s *HistoryState) buildCommittedCache() {
	if s.cacheValid && s.cachedWidth == s.renderWidth {
		return
	}
	render := make([]string, 0, len(s.committed)*4)
	anchors := make([]ScrollAnchor, 0, len(s.committed)*4)
	cells := make([]renderedCellIndex, 0, len(s.committed))
	committedLines := 0
	for index := 0; index < len(s.committed); {
		cell := s.committed[index]
		key := routineToolAggregationKey(cell)
		groupEnd := index + 1
		if key != "" {
			for groupEnd < len(s.committed) && routineToolAggregationKey(s.committed[groupEnd]) == key {
				groupEnd++
			}
		}
		if index > 0 {
			render = append(render, "")
			anchors = append(anchors, ScrollAnchor{cell: cell, cellIndex: index, line: 0, valid: true})
		}
		startLine := len(render)
		if groupEnd-index >= 2 {
			groupLines := renderRoutineToolAggregate(key, groupEnd-index, s.renderWidth)
			committedLines += len(groupLines)
			for line := range groupLines {
				anchors = append(anchors, ScrollAnchor{cell: cell, cellIndex: index, line: line, valid: true})
			}
			render = append(render, groupLines...)
			for member := index; member < groupEnd; member++ {
				memberCell := s.committed[member]
				releaseCommittedCellRenderCache(memberCell)
				cells = append(cells, renderedCellIndex{cell: memberCell, startLine: startLine, lineCount: len(groupLines)})
			}
			index = groupEnd
			continue
		}
		cellLines := renderHistoryCell(cell, s.renderWidth)
		committedLines += len(cellLines)
		for line := range cellLines {
			anchors = append(anchors, ScrollAnchor{cell: cell, cellIndex: index, line: line, valid: true})
		}
		render = append(render, cellLines...)
		releaseCommittedCellRenderCache(cell)
		cells = append(cells, renderedCellIndex{cell: cell, startLine: startLine, lineCount: len(cellLines)})
		index++
	}
	s.committedLines = committedLines
	s.cachedRender = render
	s.cachedAnchors = anchors
	s.cachedCells = cells
	s.cachedRenderText = ""
	s.renderTextValid = false
	s.cachedRawText = ""
	s.rawTextValid = false
	s.cachedWidth = s.renderWidth
	s.cacheValid = true
}

func (s *HistoryState) RenderLines() []string {
	s.buildCommittedCache()
	if s.active == nil {
		return append([]string(nil), s.cachedRender...)
	}
	activeLines := renderHistoryCell(s.active, s.renderWidth)
	out := make([]string, len(s.cachedRender), len(s.cachedRender)+len(activeLines)+1)
	copy(out, s.cachedRender)
	if len(out) > 0 {
		out = append(out, "")
	}
	return append(out, activeLines...)
}

// RenderContent renders the main transcript directly as viewport content.
// The finalized prefix is cached so streaming updates only rebuild the active tail.
func (s *HistoryState) RenderContent() string {
	if s == nil {
		return ""
	}
	s.buildCommittedCache()
	committed := s.committedRenderText()
	if s.active == nil {
		return committed
	}
	activeLines := renderHistoryCell(s.active, s.renderWidth)
	if len(activeLines) == 0 {
		return committed
	}
	activeBytes := len(activeLines) - 1
	for _, line := range activeLines {
		activeBytes += len(line)
	}
	separatorBytes := 0
	if committed != "" {
		separatorBytes = 2
	}
	var out strings.Builder
	out.Grow(len(committed) + separatorBytes + activeBytes)
	if committed != "" {
		out.WriteString(committed)
		out.WriteString("\n\n")
	}
	for index, line := range activeLines {
		if index > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	return out.String()
}

func (s *HistoryState) committedRenderText() string {
	if s.renderTextValid {
		return s.cachedRenderText
	}
	s.cachedRenderText = strings.Join(s.cachedRender, "\n")
	if len(s.cachedRender) > 0 {
		// Re-slice the joined render so the line cache and full-text cache share
		// one backing allocation instead of retaining both representations.
		s.cachedRender = strings.Split(s.cachedRenderText, "\n")
	}
	s.renderTextValid = true
	return s.cachedRenderText
}

// RenderTailContent renders only the newest rich transcript lines. It reports
// whether older lines were omitted so callers can hydrate full scrollback on demand.
func (s *HistoryState) RenderTailContent(maxLines int) (string, bool) {
	if s == nil {
		return "", false
	}
	return s.RenderTailContentAt(s.renderWidth, maxLines)
}

// RenderTailContentAt renders only the newest lines at width. It keeps the
// committed prefix cached independently for alternate-width transcript views.
func (s *HistoryState) RenderTailContentAt(width, maxLines int) (string, bool) {
	if s == nil {
		return "", false
	}
	if width <= 0 {
		width = s.renderWidth
	}
	if maxLines <= 0 {
		if width == s.renderWidth {
			return s.RenderContent(), false
		}
		return strings.Join(s.RenderLinesAt(width), "\n"), false
	}
	var committed []string
	if width == s.renderWidth {
		s.buildCommittedCache()
		committed = s.cachedRender
	} else {
		s.buildAlternateRenderCache(width)
		committed = s.altRender
	}
	var activeLines []string
	if s.active != nil {
		activeLines = renderHistoryCell(s.active, width)
	}
	separator := len(committed) > 0 && len(activeLines) > 0
	totalLines := len(committed) + len(activeLines)
	if separator {
		totalLines++
	}
	if totalLines <= maxLines {
		return joinRenderedTail(committed, separator, activeLines), false
	}

	remaining := maxLines
	activeStart := len(activeLines)
	if remaining > 0 && len(activeLines) > 0 {
		take := min(remaining, len(activeLines))
		activeStart -= take
		remaining -= take
	}
	includeSeparator := false
	if remaining > 0 && separator && activeStart == 0 {
		includeSeparator = true
		remaining--
	}
	committedStart := len(committed)
	if remaining > 0 {
		take := min(remaining, len(committed))
		committedStart -= take
	}
	return joinRenderedTail(committed[committedStart:], includeSeparator, activeLines[activeStart:]), true
}

func joinRenderedTail(committed []string, blankSeparator bool, active []string) string {
	bytes := 0
	for _, line := range committed {
		bytes += len(line)
	}
	for _, line := range active {
		bytes += len(line)
	}
	if len(committed) > 1 {
		bytes += len(committed) - 1
	}
	if len(active) > 1 {
		bytes += len(active) - 1
	}
	if blankSeparator {
		if len(committed) == 0 {
			bytes++
		} else {
			bytes += 2
		}
	}
	var out strings.Builder
	out.Grow(bytes)
	for index, line := range committed {
		if index > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	if blankSeparator {
		if len(committed) == 0 {
			out.WriteByte('\n')
		} else {
			out.WriteString("\n\n")
		}
	}
	for index, line := range active {
		if index > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	return out.String()
}

// RenderLinesAt renders rich content at a temporary width, useful for the
// narrower transcript overlay without changing the main viewport's cache.
func (s *HistoryState) RenderLinesAt(width int) []string {
	if s == nil {
		return nil
	}
	if width <= 0 {
		width = s.renderWidth
	}
	if width == s.renderWidth {
		return s.RenderLines()
	}
	s.buildAlternateRenderCache(width)
	if s.active == nil {
		return append([]string(nil), s.altRender...)
	}
	activeLines := renderHistoryCell(s.active, width)
	out := make([]string, len(s.altRender), len(s.altRender)+len(activeLines)+1)
	copy(out, s.altRender)
	if len(out) > 0 {
		out = append(out, "")
	}
	return append(out, activeLines...)
}

func (s *HistoryState) buildAlternateRenderCache(width int) {
	if s.altRenderValid && s.altRenderWidth == width {
		return
	}
	render := make([]string, 0, len(s.committed)*4)
	for index := 0; index < len(s.committed); {
		cell := s.committed[index]
		key := routineToolAggregationKey(cell)
		groupEnd := index + 1
		if key != "" {
			for groupEnd < len(s.committed) && routineToolAggregationKey(s.committed[groupEnd]) == key {
				groupEnd++
			}
		}
		if index > 0 {
			render = append(render, "")
		}
		if groupEnd-index >= 2 {
			render = append(render, renderRoutineToolAggregate(key, groupEnd-index, width)...)
			for member := index; member < groupEnd; member++ {
				releaseCommittedCellRenderCache(s.committed[member])
			}
			index = groupEnd
			continue
		}
		render = append(render, renderHistoryCell(cell, width)...)
		releaseCommittedCellRenderCache(cell)
		index++
	}
	s.altRender = render
	s.altRenderWidth = width
	s.altRenderValid = true
}

func (s *HistoryState) Raw() string {
	s.buildCommittedCache()
	committedRaw := s.committedRawText()
	if s.active == nil {
		return committedRaw
	}
	activeRaw := strings.Join(s.active.RawLines(), "\n")
	if committedRaw == "" {
		return activeRaw
	}
	if activeRaw == "" {
		return committedRaw
	}
	var out strings.Builder
	out.Grow(len(committedRaw) + 1 + len(activeRaw))
	out.WriteString(committedRaw)
	out.WriteByte('\n')
	out.WriteString(activeRaw)
	return out.String()
}

// RawTailContent returns the newest raw transcript lines without rebuilding
// the committed transcript prefix. The boolean reports omitted older lines.
func (s *HistoryState) RawTailContent(maxLines int) (string, bool) {
	if s == nil {
		return "", false
	}
	if maxLines <= 0 {
		return s.Raw(), false
	}
	committed := s.committedRawText()
	activeLines := []string(nil)
	if s.active != nil {
		activeLines = s.active.RawLines()
	}
	committedCount := 0
	if committed != "" {
		committedCount = strings.Count(committed, "\n") + 1
	}
	truncated := committedCount+len(activeLines) > maxLines
	if len(activeLines) >= maxLines {
		activeLines = activeLines[len(activeLines)-maxLines:]
		return strings.Join(activeLines, "\n"), truncated || committedCount > 0
	}
	remaining := maxLines - len(activeLines)
	committedTail := rawTextTail(committed, remaining)
	if committedTail == "" {
		return strings.Join(activeLines, "\n"), truncated
	}
	if len(activeLines) == 0 {
		return committedTail, truncated
	}
	return committedTail + "\n" + strings.Join(activeLines, "\n"), truncated
}

func rawTextTail(text string, maxLines int) string {
	text = strings.TrimRight(text, "\n")
	if text == "" || maxLines <= 0 {
		return ""
	}
	start := len(text)
	for line := 0; line < maxLines && start > 0; line++ {
		separator := strings.LastIndexByte(text[:start], '\n')
		if separator < 0 {
			start = 0
			break
		}
		start = separator
	}
	if start < len(text) && text[start] == '\n' {
		start++
	}
	return text[start:]
}

func (s *HistoryState) committedRawText() string {
	if s.rawTextValid {
		return s.cachedRawText
	}
	var out strings.Builder
	first := true
	for _, cell := range s.committed {
		for _, line := range cell.RawLines() {
			if !first {
				out.WriteByte('\n')
			}
			out.WriteString(line)
			first = false
		}
	}
	s.cachedRawText = out.String()
	s.rawTextValid = true
	return s.cachedRawText
}
