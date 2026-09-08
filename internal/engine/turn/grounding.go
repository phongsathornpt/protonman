package turn

import "github.com/phongsathornpt/protonman/internal/core/tool"

type groundingState struct {
	evidence tool.EvidenceKind
	grounded bool
	misses   int
}

func newGroundingState(evidence tool.EvidenceKind) groundingState {
	return groundingState{evidence: evidence, grounded: evidence == tool.EvidenceNone}
}

func (s groundingState) pending() bool {
	return s.evidence != tool.EvidenceNone && !s.grounded
}

func (s groundingState) filterDefinitions(definitions []tool.Definition) []tool.Definition {
	if !s.pending() {
		return append([]tool.Definition(nil), definitions...)
	}
	filtered := make([]tool.Definition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Evidence == s.evidence {
			filtered = append(filtered, definition)
		}
	}
	return filtered
}

func (s *groundingState) recordMiss() bool {
	if s == nil || !s.pending() {
		return false
	}
	s.misses++
	return s.misses >= 2
}

func (s *groundingState) observe(executions []executedCall, definitions []tool.Definition) bool {
	if s == nil || !s.pending() {
		return false
	}
	byName := make(map[string]tool.Definition, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}
	for _, execution := range executions {
		definition, ok := byName[execution.call.Name]
		if !ok || tool.EffectiveCallEvidence(definition, execution.call.Arguments) != s.evidence {
			continue
		}
		if execution.suppressed || execution.err != nil || execution.result.Denied || execution.result.Failure != nil {
			continue
		}
		s.grounded = true
		s.misses = 0
		return true
	}
	return false
}
