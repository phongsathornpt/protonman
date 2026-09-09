package history

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
	s.active = cell
	s.touchActive()
}

func (s *HistoryState) CompleteTool(completed ToolCell) {
	completed.Running = false
	s.CompleteToolCall(completed.CallID, completed.Name, &completed)
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
