package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SynthesisResult couples one immutable result with the reference that made it
// available to the parent turn.
type SynthesisResult struct {
	Ref    ResultRef `json:"ref"`
	Result Result    `json:"result"`
}

// SynthesisBatch is one deterministic delivery of previously unseen child
// results for a parent turn.
type SynthesisBatch struct {
	Turn      TurnRef           `json:"turn"`
	Cursor    EventCursor       `json:"cursor"`
	Results   []SynthesisResult `json:"results"`
	Truncated bool              `json:"truncated"`
	TimedOut  bool              `json:"timed_out"`
}

type synthesisConsumerState struct {
	mu        sync.Mutex
	cursor    EventCursor
	delivered map[ResultRef]struct{}
}

// SynthesisCoordinator converts low-level result-availability events into
// exactly-once, turn-scoped result batches for runtime integration.
type SynthesisCoordinator struct {
	source *Coordinator
	mu     sync.Mutex
	states map[string]*synthesisConsumerState
}

func NewSynthesisCoordinator(source *Coordinator) *SynthesisCoordinator {
	return &SynthesisCoordinator{source: source, states: make(map[string]*synthesisConsumerState)}
}

func (s *SynthesisCoordinator) stateFor(ref TurnRef) *synthesisConsumerState {
	key := activityScopeKey(ref.normalized())
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[key]
	if state == nil {
		state = &synthesisConsumerState{delivered: make(map[ResultRef]struct{})}
		s.states[key] = state
	}
	return state
}

// DrainReady resolves all currently retained, previously unseen results for one
// parent turn without waiting for a new event. The retained projection is the
// authoritative recovery path when delivery raced the caller between rounds.
func (s *SynthesisCoordinator) DrainReady(ref TurnRef) (SynthesisBatch, error) {
	if s == nil || s.source == nil {
		return SynthesisBatch{}, fmt.Errorf("synthesis coordinator source is required")
	}
	ref = ref.normalized()
	state := s.stateFor(ref)
	state.mu.Lock()
	defer state.mu.Unlock()
	results, err := s.resolveUnseenLocked(state, s.source.ResultRefsForTurn(ref))
	if err != nil {
		return SynthesisBatch{}, err
	}
	return SynthesisBatch{Turn: ref, Cursor: state.cursor, Results: results}, nil
}

// Drain waits for new result references and resolves each unseen result exactly
// once for this synthesis consumer. A timeout is non-fatal and returns no work.
func (s *SynthesisCoordinator) Drain(ctx context.Context, ref TurnRef, timeout time.Duration) (SynthesisBatch, error) {
	if s == nil || s.source == nil {
		return SynthesisBatch{}, fmt.Errorf("synthesis coordinator source is required")
	}
	ref = ref.normalized()
	state := s.stateFor(ref)
	state.mu.Lock()
	defer state.mu.Unlock()

	stream, err := s.source.WaitResultEventsAfter(ctx, ref, state.cursor, timeout)
	if err != nil {
		return SynthesisBatch{}, err
	}
	refs := resultRefsFromEvents(stream.Events)
	if stream.Truncated {
		refs = mergeResultRefs(refs, s.source.ResultRefsForTurn(ref))
	}

	results, err := s.resolveUnseenLocked(state, refs)
	if err != nil {
		return SynthesisBatch{}, err
	}
	state.cursor = stream.Cursor
	return SynthesisBatch{
		Turn: ref, Cursor: stream.Cursor, Results: results,
		Truncated: stream.Truncated, TimedOut: stream.TimedOut,
	}, nil
}

func resultRefsFromEvents(events []Event) []ResultRef {
	refs := make([]ResultRef, 0, len(events))
	for _, event := range events {
		if event.Kind != EventAgentResultAvailable || event.ResultVersion == 0 {
			continue
		}
		refs = append(refs, ResultRef{SessionID: event.SessionID, AgentID: event.AgentID, Version: event.ResultVersion}.normalized())
	}
	return refs
}

func mergeResultRefs(primary, fallback []ResultRef) []ResultRef {
	seen := make(map[ResultRef]struct{}, len(primary)+len(fallback))
	out := make([]ResultRef, 0, len(primary)+len(fallback))
	for _, group := range [][]ResultRef{primary, fallback} {
		for _, ref := range group {
			ref = ref.normalized()
			if !ref.valid() {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			out = append(out, ref)
		}
	}
	return out
}
func (s *SynthesisCoordinator) resolveUnseenLocked(state *synthesisConsumerState, refs []ResultRef) ([]SynthesisResult, error) {
	results := make([]SynthesisResult, 0, len(refs))
	for _, resultRef := range refs {
		resultRef = resultRef.normalized()
		if _, seen := state.delivered[resultRef]; seen {
			continue
		}
		result, ok := s.source.LookupResult(resultRef)
		if !ok {
			return nil, fmt.Errorf("subagent result %s@%d is unavailable", resultRef.AgentID, resultRef.Version)
		}
		state.delivered[resultRef] = struct{}{}
		results = append(results, SynthesisResult{Ref: resultRef, Result: result})
	}
	return results, nil
}
