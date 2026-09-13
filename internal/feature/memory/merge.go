package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
)

const memoryReplacementConfidenceSlack = 0.10

type extractionEvidence struct {
	SessionID       string
	SessionRevision uint64
	ObservedAt      time.Time
}

func mergeCandidates(ctx context.Context, repository corememory.Repository, workspaceKey string, candidates []candidate, evidence extractionEvidence, policy runtimepolicy.MemoryPolicy) error {
	if len(candidates) == 0 {
		return nil
	}
	workspaceSnapshot, err := repository.Load(ctx, corememory.ScopeWorkspace, workspaceKey)
	if err != nil {
		return err
	}
	workspaceEntries := make([]corememory.Entry, 0, len(candidates))
	globalEntries := make([]corememory.Entry, 0, len(candidates))
	promoted := make(map[string]struct{})
	for _, item := range candidates {
		scope := effectiveCandidateScope(item, workspaceSnapshot, evidence.SessionID, policy)
		entry := candidateEntry(item, scope, workspaceKey, evidence)
		if scope == corememory.ScopeGlobal {
			globalEntries = append(globalEntries, entry)
			promoted[candidateIdentity(item)] = struct{}{}
		} else {
			workspaceEntries = append(workspaceEntries, entry)
		}
	}
	if len(workspaceEntries) > 0 || len(promoted) > 0 {
		if err := repository.Update(ctx, corememory.ScopeWorkspace, workspaceKey, func(existing []corememory.Entry) ([]corememory.Entry, error) {
			filtered := existing[:0]
			for _, entry := range existing {
				if _, ok := promoted[memoryIdentity(entry.Kind, entry.Key)]; ok {
					continue
				}
				filtered = append(filtered, entry)
			}
			return mergeEntries(filtered, workspaceEntries, policy.MaxIndexEntries), nil
		}); err != nil {
			return err
		}
	}
	if len(globalEntries) > 0 {
		if err := repository.Update(ctx, corememory.ScopeGlobal, "", func(existing []corememory.Entry) ([]corememory.Entry, error) {
			return mergeEntries(existing, globalEntries, policy.MaxIndexEntries), nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func effectiveCandidateScope(item candidate, workspace []corememory.Entry, sessionID string, policy runtimepolicy.MemoryPolicy) corememory.Scope {
	if item.Scope != corememory.ScopeGlobal || item.Kind != corememory.KindPreference {
		return corememory.ScopeWorkspace
	}
	if item.Explicit && item.Confidence >= 0.90 {
		return corememory.ScopeGlobal
	}
	minimum := policy.GlobalPromotionMinSessions
	if minimum <= 1 {
		return corememory.ScopeGlobal
	}
	sessions := map[string]struct{}{sessionID: {}}
	identity := candidateIdentity(item)
	for _, entry := range workspace {
		if memoryIdentity(entry.Kind, entry.Key) != identity {
			continue
		}
		for _, ref := range entry.Evidence {
			if id := strings.TrimSpace(ref.SessionID); id != "" {
				sessions[id] = struct{}{}
			}
		}
	}
	if len(sessions) >= minimum {
		return corememory.ScopeGlobal
	}
	return corememory.ScopeWorkspace
}

func candidateEntry(item candidate, scope corememory.Scope, workspaceKey string, evidence extractionEvidence) corememory.Entry {
	boundWorkspace := ""
	if scope == corememory.ScopeWorkspace {
		boundWorkspace = workspaceKey
	}
	observedAt := evidence.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	return corememory.Entry{
		ID:           stableMemoryID(scope, boundWorkspace, item.Kind, item.Key),
		Scope:        scope,
		Kind:         item.Kind,
		Key:          strings.TrimSpace(item.Key),
		Value:        strings.TrimSpace(item.Value),
		Keywords:     append([]string(nil), item.Keywords...),
		WorkspaceKey: boundWorkspace,
		Confidence:   item.Confidence,
		Evidence: []corememory.EvidenceRef{{
			SessionID:       evidence.SessionID,
			SessionRevision: evidence.SessionRevision,
			MessageIDs:      append([]string(nil), item.MessageIDs...),
			ObservedAt:      observedAt,
		}},
		CreatedAt: observedAt,
		UpdatedAt: observedAt,
	}
}

func candidateIdentity(item candidate) string {
	return memoryIdentity(item.Kind, item.Key)
}

func memoryIdentity(kind corememory.Kind, key string) string {
	return strings.ToLower(strings.TrimSpace(string(kind))) + "|" + strings.ToLower(strings.TrimSpace(key))
}

func stableMemoryID(scope corememory.Scope, workspaceKey string, kind corememory.Kind, key string) string {
	normalized := strings.Join([]string{string(scope), strings.TrimSpace(workspaceKey), string(kind), strings.ToLower(strings.TrimSpace(key))}, "|")
	digest := sha256.Sum256([]byte(normalized))
	return "mem_" + hex.EncodeToString(digest[:12])
}

func mergeEntries(existing, incoming []corememory.Entry, limit int) []corememory.Entry {
	byID := make(map[string]corememory.Entry, len(existing)+len(incoming))
	for _, entry := range existing {
		byID[entry.ID] = entry
	}
	for _, next := range incoming {
		current, ok := byID[next.ID]
		if !ok {
			byID[next.ID] = next
			continue
		}
		newer := next.UpdatedAt.After(current.UpdatedAt)
		nearConfidence := next.Confidence >= current.Confidence-memoryReplacementConfidenceSlack
		if next.Confidence >= current.Confidence || (newer && nearConfidence && next.Confidence >= minExtractionConfidence) {
			current.Value = next.Value
			current.Confidence = next.Confidence
		}
		if current.CreatedAt.IsZero() || (!next.CreatedAt.IsZero() && next.CreatedAt.Before(current.CreatedAt)) {
			current.CreatedAt = next.CreatedAt
		}
		if newer {
			current.UpdatedAt = next.UpdatedAt
		}
		current.Keywords = unionStrings(current.Keywords, next.Keywords)
		current.Evidence = mergeEvidence(current.Evidence, next.Evidence)
		current.Supersedes = unionStrings(current.Supersedes, next.Supersedes)
		byID[next.ID] = current
	}
	entries := make([]corememory.Entry, 0, len(byID))
	for _, entry := range byID {
		entries = append(entries, entry)
	}
	if limit > 0 && len(entries) > limit {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].UsageCount != entries[j].UsageCount {
				return entries[i].UsageCount > entries[j].UsageCount
			}
			left := entries[i].LastUsedAt
			if left.IsZero() {
				left = entries[i].UpdatedAt
			}
			right := entries[j].LastUsedAt
			if right.IsZero() {
				right = entries[j].UpdatedAt
			}
			if !left.Equal(right) {
				return left.After(right)
			}
			return entries[i].ID < entries[j].ID
		})
		entries = entries[:limit]
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func unionStrings(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	out := make([]string, 0, len(left)+len(right))
	for _, values := range [][]string{left, right} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func mergeEvidence(left, right []corememory.EvidenceRef) []corememory.EvidenceRef {
	out := append([]corememory.EvidenceRef(nil), left...)
	for _, next := range right {
		matched := false
		for index := range out {
			if out[index].SessionID != next.SessionID || out[index].SessionRevision != next.SessionRevision {
				continue
			}
			out[index].MessageIDs = unionStrings(out[index].MessageIDs, next.MessageIDs)
			if next.ObservedAt.After(out[index].ObservedAt) {
				out[index].ObservedAt = next.ObservedAt
			}
			matched = true
			break
		}
		if !matched {
			out = append(out, next)
		}
	}
	return out
}
