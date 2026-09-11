package agent

import (
	"sort"
	"strings"
	"sync"
)

// ResultRef identifies one immutable result produced by a subagent lifecycle
// version. SessionID is part of the key so recovered sessions cannot collide.
type ResultRef struct {
	SessionID string `json:"session_id,omitempty"`
	AgentID   string `json:"agent_id"`
	Version   uint64 `json:"version"`
}

func (r ResultRef) normalized() ResultRef {
	r.SessionID = strings.TrimSpace(r.SessionID)
	r.AgentID = strings.TrimSpace(r.AgentID)
	return r
}

func (r ResultRef) valid() bool {
	r = r.normalized()
	return r.AgentID != "" && r.Version > 0
}

// ResultStore owns immutable subagent results independently from lifecycle
// projections. Implementations must return detached values to callers.
type ResultStore interface {
	Put(ResultRef, Result)
	Get(ResultRef) (Result, bool)
	Delete(ResultRef)
}

type memoryResultStore struct {
	mu      sync.RWMutex
	results map[ResultRef]Result
}

func newMemoryResultStore() *memoryResultStore {
	return &memoryResultStore{results: make(map[ResultRef]Result)}
}

func (s *memoryResultStore) Put(ref ResultRef, result Result) {
	if s == nil || !ref.valid() {
		return
	}
	ref = ref.normalized()
	s.mu.Lock()
	s.results[ref] = compactRetainedResult(result)
	s.mu.Unlock()
}

func (s *memoryResultStore) Get(ref ResultRef) (Result, bool) {
	if s == nil || !ref.valid() {
		return Result{}, false
	}
	ref = ref.normalized()
	s.mu.RLock()
	result, ok := s.results[ref]
	s.mu.RUnlock()
	if !ok {
		return Result{}, false
	}
	return compactRetainedResult(result), true
}

func (s *memoryResultStore) Delete(ref ResultRef) {
	if s == nil || !ref.valid() {
		return
	}
	ref = ref.normalized()
	s.mu.Lock()
	delete(s.results, ref)
	s.mu.Unlock()
}

// LookupResult returns one immutable result by versioned reference.
func (c *Coordinator) LookupResult(ref ResultRef) (Result, bool) {
	if c == nil || c.resultStore == nil {
		return Result{}, false
	}
	return c.resultStore.Get(ref)
}

// ResultRefsForTurn returns retained terminal result references in stable agent order.
func (c *Coordinator) ResultRefsForTurn(ref TurnRef) []ResultRef {
	if c == nil {
		return nil
	}
	ref = ref.normalized()
	c.pruneExpired()
	c.agentsMu.RLock()
	out := make([]ResultRef, 0, len(c.agents))
	for _, entry := range c.agents {
		if ref.SessionID != "" && entry.status.SessionID != ref.SessionID {
			continue
		}
		if ref.TurnID != "" && entry.status.ParentID != ref.TurnID {
			continue
		}
		if entry.status.State.Terminal() && entry.resultRef.valid() {
			out = append(out, entry.resultRef.normalized())
		}
	}
	c.agentsMu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].AgentID == out[j].AgentID {
			return out[i].Version < out[j].Version
		}
		return out[i].AgentID < out[j].AgentID
	})
	return out
}
