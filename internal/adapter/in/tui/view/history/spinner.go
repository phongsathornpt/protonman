package history

import "github.com/phongsathornpt/protonman/internal/feature/agent"

func (s *HistoryState) SetSpinnerFrame(frame string) bool {
	if s == nil {
		return false
	}
	s.spinnerFrame = frame
	changed := cellUsesSpinner(s.active)
	if changed {
		setCellSpinner(s.active, frame)
	}
	for index, cell := range s.committed {
		if r, ok := cell.(runningHistoryTool); ok && r.historyToolRunning() {
			setCellSpinner(cell, frame)
			if s.cacheValid && s.cachedWidth == s.renderWidth {
				s.refreshCachedCommittedCell(index)
			}
			s.invalidateAlternateRenderCache()
			changed = true
		}
	}
	return changed
}

// refreshCachedCommittedCell updates one already-indexed committed cell in
// place. Spinner frames are presentation-only and normally preserve line count,
// so they should not force a full transcript rerender on every animation tick.
func (s *HistoryState) refreshCachedCommittedCell(index int) {
	if !s.cacheValid || index < 0 || index >= len(s.committed) || index >= len(s.cachedCells) {
		return
	}
	cached := s.cachedCells[index]
	if !sameHistoryCell(cached.cell, s.committed[index]) {
		s.cacheValid = false
		return
	}
	lines := renderHistoryCell(s.committed[index], s.renderWidth)
	if len(lines) != cached.lineCount || cached.startLine < 0 || cached.startLine+cached.lineCount > len(s.cachedRender) {
		s.cacheValid = false
		return
	}
	copy(s.cachedRender[cached.startLine:cached.startLine+cached.lineCount], lines)
	s.cachedRenderText = ""
	s.renderTextValid = false
}

func cellUsesSpinner(cell HistoryCell) bool {
	switch typed := cell.(type) {
	case *ToolCell:
		return typed.Running
	case *AgentToolCell:
		return typed.Running
	case *AgentRunCell:
		return !typed.State.Terminal() && typed.State != agent.StateQueued
	case *ExecCell:
		return typed.Running
	case *PatchCell:
		return typed.Running
	case *ThinkingCell:
		return true
	default:
		return false
	}
}

func (s *HistoryState) SpinnerFrame() string {
	if s == nil {
		return ""
	}
	return s.spinnerFrame
}

func setCellSpinner(cell HistoryCell, frame string) {
	switch typed := cell.(type) {
	case *ToolCell:
		typed.Spinner = frame
	case *AgentToolCell:
		typed.Spinner = frame
	case *AgentRunCell:
		typed.Spinner = frame
	case *ExecCell:
		typed.Spinner = frame
	case *PatchCell:
		typed.Spinner = frame
	case *ThinkingCell:
		typed.Spinner = frame
	}
}
