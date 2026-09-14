package desktop

import (
	"path/filepath"
	"strings"
)

// WorkspaceGroup is a stable presentation-neutral grouping of sessions that
// share one persisted workspace identity.
type WorkspaceGroup struct {
	Key      string
	Name     string
	Sessions []SessionState
}

// GroupSessionsByWorkspace preserves first-seen group and session order while
// grouping by the durable workspace key when available.
func GroupSessionsByWorkspace(sessions []SessionState) []WorkspaceGroup {
	groups := make([]WorkspaceGroup, 0)
	index := make(map[string]int)
	for _, session := range sessions {
		key, name := workspaceIdentity(session)
		position, ok := index[key]
		if !ok {
			position = len(groups)
			index[key] = position
			groups = append(groups, WorkspaceGroup{Key: key, Name: name})
		}
		groups[position].Sessions = append(groups[position].Sessions, session)
	}
	return groups
}

func workspaceIdentity(session SessionState) (string, string) {
	key := strings.TrimSpace(session.WorkspaceKey)
	name := strings.TrimSpace(session.WorkspaceName)
	workspace := strings.TrimSpace(session.Workspace)
	if name == "" && workspace != "" {
		name = filepath.Base(filepath.Clean(workspace))
	}
	if name == "" {
		name = "Other"
	}
	if key != "" {
		return "key:" + key, name
	}
	if workspace != "" {
		return "path:" + filepath.Clean(workspace), name
	}
	return "name:" + strings.ToLower(name), name
}
