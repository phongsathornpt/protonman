package history

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type runningHistoryTool interface {
	HistoryCell
	historyToolID() string
	historyToolName() string
	historyToolRunning() bool
}

// HistoryState separates finalized transcript cells from one mutable in-flight
// cell. Renderers always see committed cells plus the live active tail.
type HistoryState struct {
	committed        []HistoryCell
	active           HistoryCell
	maxLines         int
	renderWidth      int
	cachedRender     []string
	cachedRenderText string
	renderTextValid  bool
	cachedRaw        []string
	cachedRawText    string
	rawTextValid     bool
	cacheValid       bool
	cachedWidth      int
	altRender        []string
	altRenderValid   bool
	altRenderWidth   int
	spinnerFrame     string
	committedLines   int
}

func NewHistoryState(maxLines int) *HistoryState {
	if maxLines <= 0 {
		maxLines = defaultHistoryMaxLines
	}
	return &HistoryState{committed: make([]HistoryCell, 0), maxLines: maxLines, renderWidth: defaultHistoryWidth}
}

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
	for _, cell := range s.committed {
		if r, ok := cell.(runningHistoryTool); ok && r.historyToolRunning() {
			setCellSpinner(cell, frame)
			s.cacheValid = false
			s.altRenderValid = false
			changed = true
		}
	}
	return changed
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
	if s.AgentRun(agentID) == nil {
		return false
	}
	s.cacheValid = false
	s.altRenderValid = false
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
	s.cacheValid = false
	s.altRenderValid = false
	s.trim()
}

func (s *HistoryState) StartThinking() {
	s.CommitActive()
	s.active = &ThinkingCell{Spinner: s.spinnerFrame}
}

func (s *HistoryState) AppendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if _, ok := s.active.(*ThinkingCell); ok {
		cell := &AssistantCell{}
		cell.appendDelta(delta)
		s.active = cell
		return
	}
	if assistant, ok := s.active.(*AssistantCell); ok {
		assistant.appendDelta(delta)
		return
	}
	s.CommitActive()
	cell := &AssistantCell{}
	cell.appendDelta(delta)
	s.active = cell
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
		s.cacheValid = false
		s.altRenderValid = false
		return true
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		if !runningToolMatches(s.committed[i], callID, name) {
			continue
		}
		s.committedLines -= historyCellLineCount(s.committed[i], s.renderWidth)
		s.committed = append(s.committed[:i], s.committed[i+1:]...)
		s.cacheValid = false
		s.altRenderValid = false
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
		s.cacheValid = false
		s.altRenderValid = false
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
		return
	}
	s.committed = append(s.committed, s.active)
	s.committedLines += historyCellLineCount(s.active, s.renderWidth)
	s.active = nil
	s.cacheValid = false
	s.altRenderValid = false
	s.trim()
}

func (s *HistoryState) Reset() {
	s.committed = s.committed[:0]
	s.active = nil
	s.cacheValid = false
	s.altRenderValid = false
	s.cachedRender = nil
	s.cachedRenderText = ""
	s.renderTextValid = false
	s.altRender = nil
	s.cachedRaw = nil
	s.cachedRawText = ""
	s.rawTextValid = false
	s.committedLines = 0
}

func (s *HistoryState) InvalidateCache() {
	s.cacheValid = false
	s.altRenderValid = false
}

func (s *HistoryState) buildCommittedCache() {
	if s.cacheValid && s.cachedWidth == s.renderWidth {
		return
	}
	render := make([]string, 0, len(s.committed)*4)
	raw := make([]string, 0, len(s.committed)*2)
	committedLines := 0
	for index, cell := range s.committed {
		if index > 0 {
			render = append(render, "")
		}
		cellLines := renderHistoryCell(cell, s.renderWidth)
		committedLines += len(cellLines)
		render = append(render, cellLines...)
		raw = append(raw, cell.RawLines()...)
	}
	s.committedLines = committedLines
	s.cachedRender = render
	s.cachedRenderText = ""
	s.renderTextValid = false
	s.cachedRaw = raw
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
	s.cachedRawText = strings.Join(s.cachedRaw, "\n")
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
		s.committed = s.committed[1:]
		s.committedLines -= historyCellLineCount(popped, s.renderWidth)
		s.cacheValid = false
		s.altRenderValid = false
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
