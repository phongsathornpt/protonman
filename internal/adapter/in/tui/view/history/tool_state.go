package history

import (
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

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
	Target string
}

// RunningTools returns all currently running tool calls in transcript order.
func (s *HistoryState) RunningTools() []RunningTool {
	if s == nil {
		return nil
	}
	out := make([]RunningTool, 0)
	inspect := func(cell HistoryCell) {
		if cell == nil {
			return
		}
		running, ok := cell.(runningHistoryTool)
		if !ok || !running.historyToolRunning() {
			return
		}
		out = append(out, RunningTool{CallID: running.historyToolID(), Name: running.historyToolName(), Target: runningToolTarget(cell)})
	}
	for _, cell := range s.committed {
		inspect(cell)
	}
	inspect(s.active)
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

// LastRunningTool returns the newest running tool with presentation metadata.
func (s *HistoryState) LastRunningTool() (RunningTool, bool) {
	if s == nil {
		return RunningTool{}, false
	}
	if running, ok := s.active.(runningHistoryTool); ok && running.historyToolRunning() {
		return RunningTool{CallID: running.historyToolID(), Name: running.historyToolName(), Target: runningToolTarget(s.active)}, true
	}
	for i := len(s.committed) - 1; i >= 0; i-- {
		if running, ok := s.committed[i].(runningHistoryTool); ok && running.historyToolRunning() {
			return RunningTool{CallID: running.historyToolID(), Name: running.historyToolName(), Target: runningToolTarget(s.committed[i])}, true
		}
	}
	return RunningTool{}, false
}

// LastRunningToolName returns the newest running tool name, if any.
func (s *HistoryState) LastRunningToolName() string {
	running, ok := s.LastRunningTool()
	if !ok {
		return ""
	}
	return running.Name
}

func runningToolTarget(cell HistoryCell) string {
	switch typed := cell.(type) {
	case *ToolCell:
		return typed.Target
	case *AgentToolCell:
		return typed.Target
	case *ExecCell:
		return typed.Command
	case *PatchCell:
		if len(typed.Paths) > 0 {
			return typed.Paths[0]
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

// RetryPatchCell attempts to find an existing retrying PatchCell matching the given paths.
// If found, it updates the cell for the new retry attempt in place and returns true.
func (s *HistoryState) RetryPatchCell(callID string, name string, paths []string) (*PatchCell, bool) {
	if s == nil || len(paths) == 0 {
		return nil, false
	}
	matchPaths := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	// 1. Check if active cell is a retrying PatchCell matching paths
	if patch, ok := s.active.(*PatchCell); ok && patch.Retrying && matchPaths(patch.Paths, paths) {
		if patch.Attempts < 1 {
			patch.Attempts = 1
		}
		patch.Attempts++
		patch.CallID = callID
		patch.Running = true
		patch.FailureCode = ""
		patch.LastError = ""
		patch.Body = ""
		s.touchActive()
		return patch, true
	}

	// 2. Search backwards in committed cells
	for i := len(s.committed) - 1; i >= 0; i-- {
		patch, ok := s.committed[i].(*PatchCell)
		if !ok || !patch.Retrying || !matchPaths(patch.Paths, paths) {
			continue
		}
		// If an intermediate trailing cell is a recovery read of the same file, discard it
		if len(paths) == 1 {
			targetPath := paths[0]
			for j := len(s.committed) - 1; j > i; j-- {
				if tc, ok := s.committed[j].(*ToolCell); ok && tc.Target == targetPath && (tc.ToolKind == tool.KindRead || tc.Name == "read") {
					s.committedLines -= historyCellLineCount(s.committed[j], s.renderWidth)
					copy(s.committed[j:], s.committed[j+1:])
					last := len(s.committed) - 1
					s.committed[last] = nil
					s.committed = s.committed[:last]
				}
			}
		}

		oldLines := historyCellLineCount(patch, s.renderWidth)
		if patch.Attempts < 1 {
			patch.Attempts = 1
		}
		patch.Attempts++
		patch.CallID = callID
		patch.Running = true
		patch.FailureCode = ""
		patch.LastError = ""
		patch.Body = ""
		newLines := historyCellLineCount(patch, s.renderWidth)
		s.committedLines += (newLines - oldLines)
		if s.committedLines < 0 {
			s.committedLines = 0
		}
		s.touchCommitted()
		s.cacheValid = false
		s.invalidateAlternateRenderCache()
		return patch, true
	}

	return nil, false
}

// FinalizeRetryingTools transitions any lingering retrying PatchCells to final failure.
func (s *HistoryState) FinalizeRetryingTools() {
	if s == nil {
		return
	}
	changed := false
	if patch, ok := s.active.(*PatchCell); ok && patch.Retrying {
		patch.Retrying = false
		patch.Running = false
		changed = true
	}
	for _, cell := range s.committed {
		patch, ok := cell.(*PatchCell)
		if !ok || !patch.Retrying {
			continue
		}
		oldLines := historyCellLineCount(patch, s.renderWidth)
		patch.Retrying = false
		patch.Running = false
		changed = true
		newLines := historyCellLineCount(patch, s.renderWidth)
		s.committedLines += (newLines - oldLines)
	}
	if s.committedLines < 0 {
		s.committedLines = 0
	}
	if changed {
		s.cacheValid = false
		s.invalidateAlternateRenderCache()
	}
}
