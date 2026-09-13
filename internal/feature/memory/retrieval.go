// Package memory owns durable-memory retrieval and promotion behavior.
package memory

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
)

type Query struct {
	WorkspaceKey string
	Text         string
	ActiveGoal   string
	Now          time.Time
}

type Retriever struct {
	repository corememory.Repository
	policy     runtimepolicy.MemoryPolicy
}

func NewRetriever(repository corememory.Repository, policy runtimepolicy.MemoryPolicy) *Retriever {
	if policy.MaxEntriesPerTurn <= 0 || policy.MaxContextBytes <= 0 {
		policy = runtimepolicy.DurableMemory()
	}
	return &Retriever{repository: repository, policy: policy}
}

func (r *Retriever) Retrieve(ctx context.Context, query Query) ([]corememory.Entry, error) {
	if r == nil || r.repository == nil {
		return nil, nil
	}
	now := query.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	entries := make([]corememory.Entry, 0)
	workspaceKey := strings.TrimSpace(query.WorkspaceKey)
	if workspaceKey != "" {
		workspaceEntries, err := r.repository.Load(ctx, corememory.ScopeWorkspace, workspaceKey)
		if err != nil {
			return nil, err
		}
		entries = append(entries, workspaceEntries...)
	}
	globalEntries, err := r.repository.Load(ctx, corememory.ScopeGlobal, "")
	if err != nil {
		return nil, err
	}
	entries = append(entries, globalEntries...)

	queryTokens := tokenSet(query.Text + " " + query.ActiveGoal)
	if len(queryTokens) == 0 {
		return nil, nil
	}
	type scored struct {
		entry corememory.Entry
		score float64
	}
	matches := make([]scored, 0, len(entries))
	for _, entry := range entries {
		if stale(entry, now, r.policy) {
			continue
		}
		score := relevance(entry, queryTokens)
		if score <= 0 {
			continue
		}
		if entry.Scope == corememory.ScopeWorkspace {
			score += 2
		}
		score += entry.Confidence
		if entry.UsageCount > 0 {
			score += math.Log2(float64(entry.UsageCount)+1) * 0.2
		}
		matches = append(matches, scored{entry: entry, score: score})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].entry.Scope != matches[j].entry.Scope {
			return matches[i].entry.Scope == corememory.ScopeWorkspace
		}
		if !matches[i].entry.UpdatedAt.Equal(matches[j].entry.UpdatedAt) {
			return matches[i].entry.UpdatedAt.After(matches[j].entry.UpdatedAt)
		}
		return matches[i].entry.ID < matches[j].entry.ID
	})

	selected := make([]corememory.Entry, 0, min(r.policy.MaxEntriesPerTurn, len(matches)))
	bytesUsed := 0
	for _, match := range matches {
		if len(selected) >= r.policy.MaxEntriesPerTurn {
			break
		}
		size := estimatedEntryBytes(match.entry)
		if bytesUsed+size > r.policy.MaxContextBytes {
			continue
		}
		selected = append(selected, match.entry)
		bytesUsed += size
	}
	if len(selected) > 0 {
		refs := make([]corememory.UsageRef, 0, len(selected))
		for _, entry := range selected {
			refs = append(refs, corememory.UsageRef{Scope: entry.Scope, WorkspaceKey: entry.WorkspaceKey, ID: entry.ID})
		}
		if err := r.repository.RecordUsage(ctx, refs, now); err != nil {
			slog.DebugContext(ctx, "memory usage accounting failed", "error", err)
		}
	}
	return selected, nil
}

func relevance(entry corememory.Entry, queryTokens map[string]struct{}) float64 {
	keyTokens := tokenSet(entry.Key)
	valueTokens := tokenSet(entry.Value)
	keywordTokens := tokenSet(strings.Join(entry.Keywords, " "))
	score := 0.0
	for token := range queryTokens {
		if _, ok := keyTokens[token]; ok {
			score += 3
		}
		if _, ok := keywordTokens[token]; ok {
			score += 2
		}
		if _, ok := valueTokens[token]; ok {
			score += 1
		}
	}
	return score
}

func tokenSet(value string) map[string]struct{} {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
	})
	out := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if len(part) < 2 {
			continue
		}
		out[part] = struct{}{}
	}
	return out
}

func stale(entry corememory.Entry, now time.Time, policy runtimepolicy.MemoryPolicy) bool {
	if entry.UpdatedAt.IsZero() || now.Before(entry.UpdatedAt) {
		return false
	}
	age := now.Sub(entry.UpdatedAt)
	switch entry.Kind {
	case corememory.KindRepoFact:
		return policy.StaleRepoFactAge > 0 && age > policy.StaleRepoFactAge
	case corememory.KindFailure:
		return policy.StaleFailureAge > 0 && age > policy.StaleFailureAge
	default:
		return false
	}
}

func estimatedEntryBytes(entry corememory.Entry) int {
	return len(entry.Key) + len(entry.Value) + len(strings.Join(entry.Keywords, ",")) + 96
}
