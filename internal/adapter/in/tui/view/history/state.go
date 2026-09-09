package history

import (
	"reflect"
	"strings"

	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

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
	spinnerFrame      string
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

// AgentRun returns a mutable delegated-run cell regardless of whether it is
// still the active tail or has already been committed by later root activity.
func (s *HistoryState) AgentRun(agentID string) *AgentRunCell {
	if s == nil || strings.TrimSpace(agentID) == "" {
		return nil
	}
	if cell, ok := s.active.(*AgentRunCell); ok && cell.AgentID == agentID {
		return cell
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		if cell, ok := s.committed[i].(*AgentRunCell); ok && cell.AgentID == agentID {
			return cell
		}
	}
	return nil
}

// TouchAgentRun invalidates cached rendering after an in-place lifecycle update.
func (s *HistoryState) TouchAgentRun(agentID string) bool {
	cell := s.AgentRun(agentID)
	if cell == nil {
		return false
	}
	if active, ok := s.active.(*AgentRunCell); ok && active == cell {
		s.touchActive()
	} else {
		s.touchCommitted()
	}
	s.cacheValid = false
	s.invalidateAlternateRenderCache()
	s.renderTextValid = false
	s.rawTextValid = false
	return true
}

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
	s.active = &ThinkingCell{Spinner: s.spinnerFrame}
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

func (s *HistoryState) StartTool(name string) {
	s.StartToolCell(&ToolCell{Name: name, Running: true})
}

func (s *HistoryState) StartToolCall(callID string, name string) {
	s.StartToolCell(&ToolCell{CallID: callID, Name: name, Running: true})
}

func (s *HistoryState) StartToolCell(cell HistoryCell) {
	if cell == nil {
		return
	}
	s.CommitActive()
	if s.spinnerFrame != "" {
		setCellSpinner(cell, s.spinnerFrame)
	}
	s.active = cell
	s.touchActive()
}

func (s *HistoryState) CompleteTool(completed ToolCell) {
	completed.Running = false
	s.CompleteToolCall(completed.CallID, completed.Name, &completed)
}

// CompleteToolCell is the name-based compatibility path used by legacy tests.
// Runtime tool events should use CompleteToolCall so parallel calls of the same
// tool cannot be confused.
func (s *HistoryState) CompleteToolCell(name string, completed HistoryCell) {
	s.CompleteToolCall("", name, completed)
}

func (s *HistoryState) DiscardToolCall(callID string, name string) bool {
	if s == nil {
		return false
	}
	if runningToolMatches(s.active, callID, name) {
		s.active = nil
		s.touchActive()
		s.cacheValid = false
		s.invalidateAlternateRenderCache()
		return true
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		if !runningToolMatches(s.committed[i], callID, name) {
			continue
		}
		s.committedLines -= historyCellLineCount(s.committed[i], s.renderWidth)
		copy(s.committed[i:], s.committed[i+1:])
		last := len(s.committed) - 1
		s.committed[last] = nil
		s.committed = s.committed[:last]
		s.touchCommitted()
		s.cacheValid = false
		s.invalidateAlternateRenderCache()
		s.renderTextValid = false
		s.rawTextValid = false
		return true
	}
	return false
}

func (s *HistoryState) CompleteToolCall(callID string, name string, completed HistoryCell) {
	if completed == nil {
		return
	}
	if runningToolMatches(s.active, callID, name) {
		s.active = completed
		s.CommitActive()
		return
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		if !runningToolMatches(s.committed[i], callID, name) {
			continue
		}
		s.committedLines -= historyCellLineCount(s.committed[i], s.renderWidth)
		s.committed[i] = completed
		s.committedLines += historyCellLineCount(completed, s.renderWidth)
		s.touchCommitted()
		s.cacheValid = false
		s.invalidateAlternateRenderCache()
		s.trim()
		return
	}
	s.Append(completed)
}

// RunningTool identifies one currently running tool call without exposing cell internals.
type RunningTool struct {
	CallID string
	Name   string
}

// RunningTools returns all currently running tool calls in transcript order.
func (s *HistoryState) RunningTools() []RunningTool {
	if s == nil {
		return nil
	}
	cells := s.Cells()
	out := make([]RunningTool, 0)
	for _, cell := range cells {
		running, ok := cell.(runningHistoryTool)
		if !ok || !running.historyToolRunning() {
			continue
		}
		out = append(out, RunningTool{CallID: running.historyToolID(), Name: running.historyToolName()})
	}
	return out
}

// FindRunningTool returns the newest running cell matching call ID or tool name.
func (s *HistoryState) FindRunningTool(callID, name string) HistoryCell {
	if s == nil {
		return nil
	}
	if runningToolMatches(s.active, callID, name) {
		return s.active
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		if runningToolMatches(s.committed[i], callID, name) {
			return s.committed[i]
		}
	}
	return nil
}

// LastRunningToolName returns the newest running tool name, if any.
func (s *HistoryState) LastRunningToolName() string {
	if s == nil {
		return ""
	}
	if running, ok := s.active.(runningHistoryTool); ok && running.historyToolRunning() {
		return running.historyToolName()
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		if running, ok := s.committed[i].(runningHistoryTool); ok && running.historyToolRunning() {
			return running.historyToolName()
		}
	}
	return ""
}

func runningToolMatches(cell HistoryCell, callID string, name string) bool {
	running, ok := cell.(runningHistoryTool)
	if !ok || !running.historyToolRunning() {
		return false
	}
	if callID != "" {
		return running.historyToolID() == callID
	}
	return name != "" && running.historyToolName() == name
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
	s.committed = s.committed[:0]
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

// ReleaseAlternateRenderCache drops the width-specific transcript render cache.
// The cache only exists to accelerate the transcript overlay and should not
// retain a second full rendered transcript while that overlay is closed.
func (s *HistoryState) ReleaseAlternateRenderCache() {
	if s == nil {
		return
	}
	s.invalidateAlternateRenderCache()
}

func (s *HistoryState) invalidateAlternateRenderCache() {
	s.altRender = nil
	s.altRenderValid = false
	s.altRenderWidth = 0
}

func (s *HistoryState) buildCommittedCache() {
	if s.cacheValid && s.cachedWidth == s.renderWidth {
		return
	}
	render := make([]string, 0, len(s.committed)*4)
	anchors := make([]ScrollAnchor, 0, len(s.committed)*4)
	cells := make([]renderedCellIndex, 0, len(s.committed))
	committedLines := 0
	for index, cell := range s.committed {
		if index > 0 {
			render = append(render, "")
			anchors = append(anchors, ScrollAnchor{cell: cell, cellIndex: index, line: 0, valid: true})
		}
		cellLines := renderHistoryCell(cell, s.renderWidth)
		startLine := len(render)
		committedLines += len(cellLines)
		for line := range cellLines {
			anchors = append(anchors, ScrollAnchor{cell: cell, cellIndex: index, line: line, valid: true})
		}
		render = append(render, cellLines...)
		cells = append(cells, renderedCellIndex{cell: cell, startLine: startLine, lineCount: len(cellLines)})
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
	if s == nil || maxLines <= 0 {
		return s.RenderContent(), false
	}
	s.buildCommittedCache()
	var activeLines []string
	if s.active != nil {
		activeLines = renderHistoryCell(s.active, s.renderWidth)
	}
	separator := len(s.cachedRender) > 0 && len(activeLines) > 0
	totalLines := len(s.cachedRender) + len(activeLines)
	if separator {
		totalLines++
	}
	if totalLines <= maxLines {
		return s.RenderContent(), false
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
	committedStart := len(s.cachedRender)
	if remaining > 0 {
		take := min(remaining, len(s.cachedRender))
		committedStart -= take
	}
	return joinRenderedTail(s.cachedRender[committedStart:], includeSeparator, activeLines[activeStart:]), true
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
		bytes += 2
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
		out.WriteString("\n\n")
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
	for index, cell := range s.committed {
		if index > 0 {
			render = append(render, "")
		}
		render = append(render, renderHistoryCell(cell, width)...)
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
