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
	ownedSessions := make(map[string]struct{})
	for i := range next.Sessions {
		if agentID != "" && next.Sessions[i].AgentID != agentID {
			continue
		}
		ownedSessions[next.Sessions[i].ID] = struct{}{}
		switch next.Sessions[i].Status {
		case TaskQueued, TaskRunning, TaskWaitingPermission, TaskWaitingUser:
			next.Sessions[i].Status = TaskPaused
		}
	}
	// Permission requests belong to the dead ACP process and cannot safely be
	// answered after reconnect. A resumed session may request them again.
	if agentID == "" {
		next.PermissionInbox = nil
		return next
	}
	permissions := next.PermissionInbox[:0]
	for _, permission := range next.PermissionInbox {
		if _, owned := ownedSessions[permission.SessionID]; !owned {
			permissions = append(permissions, permission)
		}
	}
	next.PermissionInbox = permissions
	return next
}
