package history

type runningHistoryTool interface {
	HistoryCell
	historyToolID() string
	historyToolName() string
	historyToolRunning() bool
}

// HistoryState separates finalized transcript cells from one mutable in-flight
// cell. Renderers always see committed cells plus the live active tail.
type HistoryState struct {
	committed         []HistoryCell
	active            HistoryCell
	maxLines          int
	renderWidth       int
	cachedRender      []string
	cachedAnchors     []ScrollAnchor
	cachedCells       []renderedCellIndex
	cachedRenderText  string
	renderTextValid   bool
	cachedRawText     string
	rawTextValid      bool
	cacheValid        bool
	cachedWidth       int
	altRender         []string
	altRenderValid    bool
	altRenderWidth    int
	committedLines    int
	committedRevision uint64
	activeRevision    uint64
}

func NewHistoryState(maxLines int) *HistoryState {
	if maxLines <= 0 {
		maxLines = defaultHistoryMaxLines
	}
	return &HistoryState{committed: make([]HistoryCell, 0), maxLines: maxLines, renderWidth: defaultHistoryWidth}
}

// Revisions reports visual generations for the committed prefix and mutable
// active tail. Callers use them to avoid rebuilding full scrollback when only
// off-screen streaming content changed.
func (s *HistoryState) Revisions() (committed uint64, active uint64) {
	if s == nil {
		return 0, 0
	}
	return s.committedRevision, s.activeRevision
}

func (s *HistoryState) touchCommitted() { s.committedRevision++ }
func (s *HistoryState) touchActive()    { s.activeRevision++ }

// SetWidth updates the rich transcript width and invalidates visual caches.
// Raw transcript consumers remain independent of terminal dimensions.
func (s *HistoryState) SetWidth(width int) {
	if s == nil {
		return
	}
	if width <= 0 {
		width = defaultHistoryWidth
	}
	if s.renderWidth == width {
		return
	}
	s.renderWidth = width
	s.touchCommitted()
	s.touchActive()
	s.cacheValid = false
	s.buildCommittedCache()
}

func (s *HistoryState) Cells() []HistoryCell {
	cells := append([]HistoryCell{}, s.committed...)
	if s.active != nil {
		cells = append(cells, s.active)
	}
	return cells
}

func (s *HistoryState) Committed() []HistoryCell {
	return append([]HistoryCell{}, s.committed...)
}

func (s *HistoryState) Active() HistoryCell { return s.active }

func (s *HistoryState) Append(cell HistoryCell) {
	if cell == nil {
		return
	}
	s.CommitActive()
	s.committed = append(s.committed, cell)
	s.committedLines += historyCellLineCount(cell, s.renderWidth)
	s.touchCommitted()
	s.cacheValid = false
	s.invalidateAlternateRenderCache()
	s.trim()
}

func (s *HistoryState) StartThinking() {
	s.CommitActive()
	s.active = &ThinkingCell{}
	s.touchActive()
}

func (s *HistoryState) AppendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if _, ok := s.active.(*ThinkingCell); ok {
		cell := &AssistantCell{}
		cell.appendDelta(delta)
		s.active = cell
		s.touchActive()
		return
	}
	if assistant, ok := s.active.(*AssistantCell); ok {
		assistant.appendDelta(delta)
		s.touchActive()
		return
	}
	s.CommitActive()
	cell := &AssistantCell{}
	cell.appendDelta(delta)
	s.active = cell
	s.touchActive()
}

func (s *HistoryState) CommitActive() {
	if s.active == nil {
		return
	}
	if assistant, ok := s.active.(*AssistantCell); ok {
		assistant.sealStream()
	}
	if _, ok := s.active.(*ThinkingCell); ok {
		s.active = nil
		s.touchActive()
		return
	}
	s.committed = append(s.committed, s.active)
	s.committedLines += historyCellLineCount(s.active, s.renderWidth)
	s.active = nil
	s.touchCommitted()
	s.touchActive()
	s.cacheValid = false
	s.invalidateAlternateRenderCache()
	s.trim()
}

func (s *HistoryState) Reset() {
	s.committed = nil
	s.active = nil
	s.touchCommitted()
	s.touchActive()
	s.cacheValid = false
	s.invalidateAlternateRenderCache()
	s.cachedRender = nil
	s.cachedAnchors = nil
	s.cachedCells = nil
	s.cachedRenderText = ""
	s.renderTextValid = false
	s.altRender = nil
	s.cachedRawText = ""
	s.rawTextValid = false
	s.committedLines = 0
}

func (s *HistoryState) InvalidateCache() {
	s.touchCommitted()
	s.cacheValid = false
	s.invalidateAlternateRenderCache()
}

func (s *HistoryState) trim() {
	activeCount := 0
	if s.active != nil {
		activeCount = historyCellLineCount(s.active, s.renderWidth)
	}
	for s.committedLines+activeCount > s.maxLines && len(s.committed) > 1 {
		popped := s.committed[0]
		s.committed[0] = nil
		s.committed = s.committed[1:]
		s.committedLines -= historyCellLineCount(popped, s.renderWidth)
		s.cacheValid = false
		s.invalidateAlternateRenderCache()
	}
	if s.committedLines < 0 {
		s.committedLines = 0
	}
}

// LineCount returns the rendered line count at the state's current width.
func (s *HistoryState) LineCount() int { return s.lineCount() }

func (s *HistoryState) lineCount() int {
	total := s.committedLines
	if s.active != nil {
		total += historyCellLineCount(s.active, s.renderWidth)
	}
	return total
}
