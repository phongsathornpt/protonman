// Package memory defines provider-neutral durable memory contracts and policies.
package memory

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidEntry = errors.New("invalid memory entry")

type Scope string

const (
	ScopeGlobal    Scope = "global"
	ScopeWorkspace Scope = "workspace"
)

type Kind string

const (
	KindPreference Kind = "preference"
	KindRepoFact   Kind = "repo_fact"
	KindProcedure  Kind = "procedure"
	KindFailure    Kind = "failure"
	KindDecision   Kind = "decision"
)

// EvidenceRef points back to persisted session evidence without copying the
// complete source transcript into durable memory.
type EvidenceRef struct {
	SessionID       string    `json:"session_id"`
	SessionRevision uint64    `json:"session_revision,omitempty"`
	MessageIDs      []string  `json:"message_ids,omitempty"`
	ObservedAt      time.Time `json:"observed_at,omitempty"`
}

// Entry is one durable memory fact. Historical memory is supporting evidence,
// not instruction authority; callers must re-verify facts that may have drifted.
type Entry struct {
	ID           string        `json:"id"`
	Scope        Scope         `json:"scope"`
	Kind         Kind          `json:"kind"`
	Key          string        `json:"key"`
	Value        string        `json:"value"`
	Keywords     []string      `json:"keywords,omitempty"`
	WorkspaceKey string        `json:"workspace_key,omitempty"`
	Confidence   float64       `json:"confidence"`
	Evidence     []EvidenceRef `json:"evidence,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	LastUsedAt   time.Time     `json:"last_used_at,omitempty"`
	UsageCount   uint64        `json:"usage_count,omitempty"`
	Supersedes   []string      `json:"supersedes,omitempty"`
}

func (e Entry) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidEntry)
	}
	if !e.Scope.Valid() {
		return fmt.Errorf("%w: unsupported scope %q", ErrInvalidEntry, e.Scope)
	}
	if !e.Kind.Valid() {
		return fmt.Errorf("%w: unsupported kind %q", ErrInvalidEntry, e.Kind)
	}
	if strings.TrimSpace(e.Key) == "" {
		return fmt.Errorf("%w: key is required", ErrInvalidEntry)
	}
	if strings.TrimSpace(e.Value) == "" {
		return fmt.Errorf("%w: value is required", ErrInvalidEntry)
	}
	workspaceKey := strings.TrimSpace(e.WorkspaceKey)
	if e.Scope == ScopeWorkspace && workspaceKey == "" {
		return fmt.Errorf("%w: workspace scope requires workspace_key", ErrInvalidEntry)
	}
	if e.Scope == ScopeGlobal && workspaceKey != "" {
		return fmt.Errorf("%w: global scope cannot bind workspace_key", ErrInvalidEntry)
	}
	if e.Confidence < 0 || e.Confidence > 1 {
		return fmt.Errorf("%w: confidence must be between 0 and 1", ErrInvalidEntry)
	}
	return nil
}

func (s Scope) Valid() bool {
	return s == ScopeGlobal || s == ScopeWorkspace
}

func (k Kind) Valid() bool {
	switch k {
	case KindPreference, KindRepoFact, KindProcedure, KindFailure, KindDecision:
		return true
	default:
		return false
	}
}
