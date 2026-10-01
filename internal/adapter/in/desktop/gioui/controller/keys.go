//go:build desktop || desktop_gio

package controller

import (
	"strconv"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// SessionRefStorageKey builds the composite storage key for a session reference.
// The agent ID is length-prefixed so two agents can never produce the same key
// for different sessions. The shell uses the same key to scope composer drafts.
func SessionRefStorageKey(ref desktopstate.SessionRef) string {
	if ref.AgentID == "" {
		return ref.SessionID
	}
	key := make([]byte, 0, len(ref.AgentID)+len(ref.SessionID)+42)
	key = strconv.AppendInt(key, int64(len(ref.AgentID)), 10)
	key = append(key, ':')
	key = append(key, ref.AgentID...)
	key = strconv.AppendInt(key, int64(len(ref.SessionID)), 10)
	key = append(key, ':')
	key = append(key, ref.SessionID...)
	return string(key)
}

// MaxMessageStreamBytes bounds a single in-flight streaming buffer. Shell-side
// benchmarks size worst-case fixtures against this bound instead of repeating
// the literal.
const MaxMessageStreamBytes = maxMessageStreamBytes
