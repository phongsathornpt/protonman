package history

import (
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

// CollapsibleCells returns all collapsible cells in chronological order.
func (s *HistoryState) CollapsibleCells() []CollapsibleCell {
	if s == nil {
		return nil
	}
	var out []CollapsibleCell
	for _, cell := range s.committed {
		if c, ok := cell.(CollapsibleCell); ok && c.CanExpand() {
			out = append(out, c)
		}
	}
	if s.active != nil {
		if c, ok := s.active.(CollapsibleCell); ok && c.CanExpand() {
			out = append(out, c)
		}
	}
	return out
}

// CollapsibleCellAtLine returns the collapsible cell at the specified rendered line index.
func (s *HistoryState) CollapsibleCellAtLine(renderedLine int) CollapsibleCell {
	if s == nil {
		return nil
	}
	anchor := s.CaptureScrollAnchor(renderedLine)
	if !anchor.valid || anchor.cell == nil {
		return nil
	}
	if collapsible, ok := anchor.cell.(CollapsibleCell); ok && collapsible.CanExpand() {
		return collapsible
	}
	return nil
}

// SetHighlightedCell updates which collapsible cell is highlighted in navigation mode.
func (s *HistoryState) SetHighlightedCell(target CollapsibleCell) {
	if s == nil {
		return
	}
	setHighlight := func(cell HistoryCell) {
		switch c := cell.(type) {
		case *ReasoningCell:
			c.Highlighted = (target != nil && c == target)
		case *PatchCell:
			c.Highlighted = (target != nil && c == target)
		case *ExecCell:
			c.Highlighted = (target != nil && c == target)
		case *AgentRunCell:
			c.Highlighted = (target != nil && c == target)
		}
	}
	for _, cell := range s.committed {
		setHighlight(cell)
	}
	if s.active != nil {
		setHighlight(s.active)
	}
	s.cacheValid = false
	s.cachedRenderText = ""
	s.renderTextValid = false
}

// ActiveReasoning returns the currently active ReasoningCell, if any.
func (s *HistoryState) ActiveReasoning() *ReasoningCell {
	if s == nil {
		return nil
	}
	if r, ok := s.active.(*ReasoningCell); ok {
		return r
	}
	return nil
}

// StartReasoning creates or appends to an active ReasoningCell.
func (s *HistoryState) StartReasoning(spinner string, icons tuistyle.IconSet) *ReasoningCell {
	if s == nil {
		return nil
	}
	if r, ok := s.active.(*ReasoningCell); ok && r.Streaming {
		return r
	}
	// Commit any existing active cell first
	s.CommitActive()
	cell := &ReasoningCell{
		Streaming: true,
		Expanded:  true,
		Spinner:   spinner,
		Icons:     icons,
	}
	s.active = cell
	s.touchActive()
	s.InvalidateCache()
	return cell
}
