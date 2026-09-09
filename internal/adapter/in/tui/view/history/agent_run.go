package history

import "strings"

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
