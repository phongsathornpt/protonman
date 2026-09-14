package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
)

// MemoryEntry is the application-facing read model for one durable memory.
type MemoryEntry struct {
	ID         string
	Scope      string
	Kind       string
	Key        string
	Value      string
	Confidence float64
	UpdatedAt  time.Time
	UsageCount uint64
}

// MemorySnapshot is the read-only durable memory projection used by inbound
// adapters. Mutation/extraction policy remains owned by the memory subsystem.
type MemorySnapshot struct {
	WorkspaceKey string
	Workspace    []MemoryEntry
	Global       []MemoryEntry
}

// MemoryScope identifies which durable memory index a forget request targets.
type MemoryScope string

const (
	// MemoryScopeWorkspace targets the memory bound to one workspace key.
	MemoryScopeWorkspace MemoryScope = "workspace"
	// MemoryScopeGlobal targets cross-project user preferences.
	MemoryScopeGlobal MemoryScope = "global"
)

// MemoryForgetRequest names the entries to remove. WorkspaceKey is supplied by
// the caller's trusted session context, never by an untrusted client payload.
type MemoryForgetRequest struct {
	Scope        MemoryScope
	WorkspaceKey string
	IDs          []string
}

// MemoryForgetResult reports how many entries were removed.
type MemoryForgetResult struct {
	Scope   MemoryScope
	Removed int
}

// ErrMemoryForgetDenied reports a forget request that failed the authority check.
var ErrMemoryForgetDenied = errors.New("memory forget denied")

// maxForgetIDs bounds one forget request so a malformed payload cannot force an
// unbounded scan or an oversized index rewrite.
const maxForgetIDs = 256

// Memories exposes durable memory inspection and correction without leaking the
// persistence adapter into inbound clients.
type Memories struct {
	repository corememory.Repository
}

func NewMemories(repository corememory.Repository) *Memories {
	if repository == nil {
		return nil
	}
	return &Memories{repository: repository}
}

// Inspect loads workspace-local and global memory independently. Historical
// memory remains supporting evidence only; this method does not record usage or
// trigger extraction because merely opening an inspector must be side-effect free.
func (m *Memories) Inspect(ctx context.Context, workspaceKey string) (MemorySnapshot, error) {
	if m == nil || m.repository == nil {
		return MemorySnapshot{}, fmt.Errorf("memory repository is unavailable")
	}
	workspaceKey = strings.TrimSpace(workspaceKey)
	result := MemorySnapshot{WorkspaceKey: workspaceKey}
	if workspaceKey != "" {
		entries, err := m.repository.Load(ctx, corememory.ScopeWorkspace, workspaceKey)
		if err != nil {
			return MemorySnapshot{}, fmt.Errorf("load workspace memory: %w", err)
		}
		result.Workspace = projectMemoryEntries(entries)
	}
	entries, err := m.repository.Load(ctx, corememory.ScopeGlobal, "")
	if err != nil {
		return MemorySnapshot{}, fmt.Errorf("load global memory: %w", err)
	}
	result.Global = projectMemoryEntries(entries)
	return result, nil
}

// Forget permanently removes durable memory entries so a wrong or harmful fact
// can be corrected instead of persisting until it ages out of retrieval.
//
// Authority is fail-closed. A workspace forget requires the caller's trusted
// workspace key: a blank key is rejected rather than widened to "every
// workspace". Global scope is a separate, explicitly requested capability
// because it changes cross-project behavior.
func (m *Memories) Forget(ctx context.Context, request MemoryForgetRequest) (MemoryForgetResult, error) {
	if m == nil || m.repository == nil {
		return MemoryForgetResult{}, fmt.Errorf("memory repository is unavailable")
	}
	result := MemoryForgetResult{Scope: request.Scope}
	ids := normalizeForgetIDs(request.IDs)
	if len(ids) == 0 {
		return result, fmt.Errorf("%w: at least one memory id is required", ErrMemoryForgetDenied)
	}
	if len(ids) > maxForgetIDs {
		return result, fmt.Errorf("%w: at most %d ids may be forgotten in one request", ErrMemoryForgetDenied, maxForgetIDs)
	}

	var scope corememory.Scope
	var workspaceKey string
	switch request.Scope {
	case MemoryScopeWorkspace:
		workspaceKey = strings.TrimSpace(request.WorkspaceKey)
		if workspaceKey == "" {
			return result, fmt.Errorf("%w: workspace key is required to forget workspace memory", ErrMemoryForgetDenied)
		}
		scope = corememory.ScopeWorkspace
	case MemoryScopeGlobal:
		if strings.TrimSpace(request.WorkspaceKey) != "" {
			return result, fmt.Errorf("%w: global memory scope cannot bind a workspace key", ErrMemoryForgetDenied)
		}
		scope = corememory.ScopeGlobal
	default:
		return result, fmt.Errorf("%w: unsupported memory scope %q", ErrMemoryForgetDenied, request.Scope)
	}

	removed, err := m.repository.Forget(ctx, scope, workspaceKey, ids)
	if err != nil {
		return result, fmt.Errorf("forget %s memory: %w", scope, err)
	}
	result.Removed = removed
	return result, nil
}

func normalizeForgetIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func projectMemoryEntries(entries []corememory.Entry) []MemoryEntry {
	out := make([]MemoryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, MemoryEntry{
			ID:         entry.ID,
			Scope:      string(entry.Scope),
			Kind:       string(entry.Kind),
			Key:        entry.Key,
			Value:      entry.Value,
			Confidence: entry.Confidence,
			UpdatedAt:  entry.UpdatedAt,
			UsageCount: entry.UsageCount,
		})
	}
	return out
}
