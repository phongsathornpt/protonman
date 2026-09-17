package desktop

// MarkDisconnected preserves visible session/timeline/subagent state while
// converting in-flight work into a paused state. In-flight prompts are not
// replayed automatically after reconnect because tool side effects may already
// have occurred before the transport died.
func MarkDisconnected(current State) State {
	return MarkAgentDisconnected(current, "")
}

// MarkAgentDisconnected pauses only sessions owned by the disconnected ACP
// process. Other agents may continue serving their sessions.
func MarkAgentDisconnected(current State, agentID string) State {
	next := cloneState(current)
	for i := range next.Sessions {
		if agentID != "" && next.Sessions[i].AgentID != agentID {
			continue
		}
		switch next.Sessions[i].Status {
		case TaskQueued, TaskRunning, TaskWaitingPermission, TaskWaitingUser:
			next.Sessions[i].Status = TaskPaused
		}
	}
	// Permission requests belong to the dead ACP process and cannot safely be
	// answered after reconnect. A resumed session may request them again.
	next.PermissionInbox = nil
	if agentID != "" && next.AgentHealth != nil {
		if health, ok := next.AgentHealth[agentID]; ok {
			health.Status = AgentStatusDisconnected
			next.AgentHealth[agentID] = health
		}
	}
	return next
}
