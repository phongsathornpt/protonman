package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"
)

const (
	StateFileName = "state.json"
	TodoFileName  = "todo.md"
)

// Resources are the durable files owned by one Protonman session.
type Resources struct {
	Root  string
	State string
	Todo  string
}

// ResolveResources returns paths below sessionsRoot for a validated session ID.
func ResolveResources(sessionsRoot, sessionID string) (Resources, error) {
	if err := ValidateID(sessionID); err != nil {
		return Resources{}, err
	}
	root := filepath.Join(sessionsRoot, sessionID)
	return Resources{Root: root, State: filepath.Join(root, StateFileName), Todo: filepath.Join(root, TodoFileName)}, nil
}

// WorkspaceKey returns a stable, non-reversible key for an absolute or logical workspace path.
func WorkspaceKey(workDir string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(workDir)))
	return hex.EncodeToString(digest[:8])
}

// NewID generates a collision-resistant session ID bound to a workspace key.
func NewID(workDir string) string {
	now := time.Now().UTC()
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		fallback := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", workDir, now.UnixNano())))
		copy(entropy[:], fallback[:len(entropy)])
	}
	return fmt.Sprintf("workspace-%s-%s-%s", WorkspaceKey(workDir), now.Format("20060102-150405.000000000"), hex.EncodeToString(entropy[:]))
}
