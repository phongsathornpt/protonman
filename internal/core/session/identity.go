package session

import "strings"

// WorkspaceKeyFromID returns the workspace key embedded in Protonman-generated
// session IDs. Custom session IDs intentionally return an empty value so callers
// do not invent project affinity from unrelated identifiers.
func WorkspaceKeyFromID(sessionID string) string {
	const prefix = "workspace-"
	value := strings.TrimSpace(sessionID)
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(value, prefix)
	separator := strings.IndexByte(rest, '-')
	if separator <= 0 {
		return ""
	}
	return rest[:separator]
}
