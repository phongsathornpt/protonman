package history

import "reflect"

// ScrollAnchor identifies a logical rendered line inside a history cell. It is
// intentionally tied to the in-memory cell instance so live mutations can
// expand or contract earlier cells without moving the user's reading position.
type ScrollAnchor struct {
	cell      HistoryCell
	cellIndex int
	line      int
	valid     bool
}

// CaptureScrollAnchor maps a rendered history line to its owning cell. Blank
// separators anchor to the following cell so separator growth never becomes a
// visible jump. Committed lines reuse the render index built with the transcript
// cache; only the mutable active tail may need rendering here.
func (s *HistoryState) CaptureScrollAnchor(renderedLine int) ScrollAnchor {
	if s == nil || renderedLine < 0 {
		return ScrollAnchor{}
	}
	s.buildCommittedCache()
	if renderedLine < len(s.cachedAnchors) {
		return s.cachedAnchors[renderedLine]
	}
	if s.active == nil {
		return ScrollAnchor{}
	}
	cursor := len(s.cachedAnchors)
	activeIndex := len(s.committed)
	if len(s.cachedRender) > 0 {
		if renderedLine == cursor {
			return ScrollAnchor{cell: s.active, cellIndex: activeIndex, line: 0, valid: true}
		}
		cursor++
	}
	activeLines := renderHistoryCell(s.active, s.renderWidth)
	if renderedLine < cursor+len(activeLines) {
		return ScrollAnchor{cell: s.active, cellIndex: activeIndex, line: renderedLine - cursor, valid: true}
	}
	return ScrollAnchor{}
}

// ScrollAnchors returns a line-for-line logical map for RenderContent.
// The committed prefix is cached with rendered content, avoiding a second full
// render solely to reconstruct scroll metadata.
func (s *HistoryState) ScrollAnchors() []ScrollAnchor {
	if s == nil {
		return nil
	}
	s.buildCommittedCache()
	activeLines := []string(nil)
	if s.active != nil {
		activeLines = renderHistoryCell(s.active, s.renderWidth)
	}
	extra := len(activeLines)
	if len(s.cachedRender) > 0 && len(activeLines) > 0 {
		extra++
	}
	anchors := make([]ScrollAnchor, len(s.cachedAnchors), len(s.cachedAnchors)+extra)
	copy(anchors, s.cachedAnchors)
	if len(activeLines) == 0 {
		return anchors
	}
	activeIndex := len(s.committed)
	if len(s.cachedRender) > 0 {
		anchors = append(anchors, ScrollAnchor{cell: s.active, cellIndex: activeIndex, line: 0, valid: true})
	}
	for line := range activeLines {
		anchors = append(anchors, ScrollAnchor{cell: s.active, cellIndex: activeIndex, line: line, valid: true})
	}
	return anchors
}

// ResolveScrollAnchor returns the current rendered line for a previously
// captured anchor after live history cells have changed size. Committed cell
// positions come from the render cache instead of rendering every preceding cell.
func (s *HistoryState) ResolveScrollAnchor(anchor ScrollAnchor) (int, bool) {
	if s == nil || !anchor.valid {
		return 0, false
	}
	s.buildCommittedCache()
	if anchor.cellIndex >= 0 && anchor.cellIndex < len(s.cachedCells) {
		indexed := s.cachedCells[anchor.cellIndex]
		if sameHistoryCell(indexed.cell, anchor.cell) {
			return resolveIndexedAnchor(indexed, anchor.line), true
		}
	}
	for _, indexed := range s.cachedCells {
		if sameHistoryCell(indexed.cell, anchor.cell) {
			return resolveIndexedAnchor(indexed, anchor.line), true
		}
	}
	if s.active != nil && sameHistoryCell(s.active, anchor.cell) {
		start := len(s.cachedRender)
		if start > 0 {
			start++
		}
		lines := renderHistoryCell(s.active, s.renderWidth)
		if len(lines) == 0 {
			return start, true
		}
		line := min(anchor.line, len(lines)-1)
		return start + line, true
	}
	if anchor.cellIndex >= 0 && anchor.cellIndex < len(s.cachedCells) {
		return s.cachedCells[anchor.cellIndex].startLine, true
	}
	if anchor.cellIndex == len(s.committed) && s.active != nil {
		start := len(s.cachedRender)
		if start > 0 {
			start++
		}
		return start, true
	}
	return 0, false
}

type renderedCellIndex struct {
	cell      HistoryCell
	startLine int
	lineCount int
}

func resolveIndexedAnchor(index renderedCellIndex, line int) int {
	if index.lineCount <= 0 {
		return index.startLine
	}
	return index.startLine + min(line, index.lineCount-1)
}

func sameHistoryCell(left, right HistoryCell) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return reflect.ValueOf(left).Kind() == reflect.Pointer &&
		reflect.ValueOf(right).Kind() == reflect.Pointer &&
		reflect.ValueOf(left).Pointer() == reflect.ValueOf(right).Pointer()
}
