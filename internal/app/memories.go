package app

import (
	"context"
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

// Memories exposes read-only durable memory inspection without leaking the
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
