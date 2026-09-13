package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/session"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const minExtractionConfidence = 0.65

// Extractor converts a bounded set of idle prior root sessions into durable
// memory. It never processes the currently active session.
type Extractor struct {
	sessions         session.Repository
	memories         corememory.Repository
	model            sdk.LanguageModel
	currentSessionID string
	workspaceKey     string
	policy           runtimepolicy.MemoryPolicy
	now              func() time.Time
}

func NewExtractor(sessions session.Repository, memories corememory.Repository, model sdk.LanguageModel, currentSessionID, workspaceKey string, policy runtimepolicy.MemoryPolicy) *Extractor {
	if policy.MaxExtractionSessions <= 0 || policy.MaxExtractionInputBytes <= 0 {
		policy = runtimepolicy.DurableMemory()
	}
	return &Extractor{
		sessions:         sessions,
		memories:         memories,
		model:            model,
		currentSessionID: strings.TrimSpace(currentSessionID),
		workspaceKey:     strings.TrimSpace(workspaceKey),
		policy:           policy,
		now:              func() time.Time { return time.Now().UTC() },
	}
}

func (e *Extractor) Run(ctx context.Context) error {
	if e == nil || e.sessions == nil || e.memories == nil || e.model == nil {
		return nil
	}
	if e.workspaceKey == "" {
		return nil
	}
	now := e.now()
	scanLimit := e.policy.MaxExtractionSessions*4 + 1
	summaries, err := e.sessions.ListSummaries(ctx, session.ListOptions{WorkspaceKey: e.workspaceKey, Limit: scanLimit})
	if err != nil {
		return fmt.Errorf("list sessions for memory extraction: %w", err)
	}
	processed := 0
	for _, summary := range summaries {
		if processed >= e.policy.MaxExtractionSessions {
			break
		}
		if summary.ID == e.currentSessionID || strings.TrimSpace(summary.ID) == "" {
			continue
		}
		if e.policy.ExtractionIdleAge > 0 && !summary.UpdatedAt.IsZero() && now.Sub(summary.UpdatedAt) < e.policy.ExtractionIdleAge {
			continue
		}
		state, found, loadErr := e.sessions.Load(ctx, summary.ID)
		if loadErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("load session %q for memory extraction: %w", summary.ID, ctxErr)
			}
			slog.WarnContext(ctx, "skip session after memory extraction load failure", "session_id", summary.ID, "error", loadErr)
			continue
		}
		if !found || (state.WorkspaceKey != "" && state.WorkspaceKey != e.workspaceKey) {
			continue
		}
		revision, alreadyProcessed, revisionErr := e.memories.ProcessedRevision(ctx, summary.ID)
		if revisionErr != nil {
			return fmt.Errorf("read processed memory revision for %q: %w", summary.ID, revisionErr)
		}
		if alreadyProcessed && revision >= state.Revision {
			continue
		}
		transcript := buildExtractionTranscript(summary.ID, state, e.policy.MaxExtractionInputBytes)
		if transcript == "" {
			if err := e.memories.MarkProcessed(ctx, summary.ID, state.Revision); err != nil {
				return fmt.Errorf("mark empty session %q processed: %w", summary.ID, err)
			}
			processed++
			continue
		}
		candidates, extractErr := runExtractionModel(ctx, e.model, transcript)
		if extractErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("extract memory from session %q: %w", summary.ID, ctxErr)
			}
			slog.WarnContext(ctx, "skip session after memory extraction failure", "session_id", summary.ID, "revision", state.Revision, "error", extractErr)
			continue
		}
		candidates = validateCandidateEvidence(candidates, state)
		if err := mergeCandidates(ctx, e.memories, e.workspaceKey, candidates, extractionEvidence{
			SessionID:       summary.ID,
			SessionRevision: state.Revision,
			ObservedAt:      state.UpdatedAt,
		}, e.policy); err != nil {
			return fmt.Errorf("merge memory from session %q: %w", summary.ID, err)
		}
		if err := e.memories.MarkProcessed(ctx, summary.ID, state.Revision); err != nil {
			return fmt.Errorf("mark session %q processed: %w", summary.ID, err)
		}
		processed++
	}
	return nil
}

func (e *Extractor) StartBackground() {
	if e == nil {
		return
	}
	timeout := e.policy.ExtractionTimeout
	if timeout <= 0 {
		timeout = runtimepolicy.DurableMemory().ExtractionTimeout
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := e.Run(ctx); err != nil {
			slog.Debug("background memory extraction stopped", "error", err)
		}
	}()
}

func validateCandidateEvidence(candidates []candidate, state session.State) []candidate {
	validIDs := make(map[string]struct{}, len(state.Messages))
	for _, message := range state.Messages {
		if id := strings.TrimSpace(message.ID); id != "" {
			validIDs[id] = struct{}{}
		}
	}
	out := make([]candidate, 0, len(candidates))
	for _, item := range candidates {
		if item.Confidence < minExtractionConfidence {
			continue
		}
		ids := make([]string, 0, len(item.MessageIDs))
		seen := make(map[string]struct{}, len(item.MessageIDs))
		for _, id := range item.MessageIDs {
			id = strings.TrimSpace(id)
			if _, ok := validIDs[id]; !ok {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			continue
		}
		item.MessageIDs = ids
		out = append(out, item)
	}
	return out
}
