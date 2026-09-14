package desktop

// MarkDisconnected preserves visible session/timeline/subagent state while
// converting in-flight work into a paused state. In-flight prompts are not
// replayed automatically after reconnect because tool side effects may already
// have occurred before the transport died.
func MarkDisconnected(current State) State {
	next := cloneState(current)
	for i := range next.Sessions {
		switch next.Sessions[i].Status {
		case TaskQueued, TaskRunning, TaskWaitingPermission, TaskWaitingUser:
			next.Sessions[i].Status = TaskPaused
		}
	}
	// Permission requests belong to the dead ACP process and cannot safely be
	// answered after reconnect. A resumed session may request them again.
	next.PermissionInbox = nil
	return next
}
