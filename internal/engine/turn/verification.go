package turn

import (
	"encoding/json"

	"github.com/phongsathornpt/proton/internal/core/tool"
)

// VerificationState records whether successful workspace mutations have been
// empirically checked after the most recent mutation in this turn.
type VerificationState struct {
	Mutated  bool   `json:"mutated"`
	Verified bool   `json:"verified"`
	Verifier string `json:"verifier,omitempty"`
}

func (s *VerificationState) observe(executions []executedCall, definitions []tool.Definition) {
	defs := make(map[string]tool.Definition, len(definitions))
	for _, definition := range definitions {
		defs[definition.Name] = definition
	}
	for _, execution := range executions {
		if execution.err != nil || execution.result.Denied || execution.result.Failure != nil {
			continue
		}
		if verifier, ok := verificationCall(execution.call); ok {
			if s.Mutated {
				s.Verified = true
				s.Verifier = verifier
			}
			continue
		}
		definition, ok := defs[execution.call.Name]
		if !ok || tool.EffectiveCallMutability(definition, execution.call.Arguments) != tool.MutabilityReadOnly {
			s.Mutated = true
			s.Verified = false
			s.Verifier = ""
		}
	}
}

func verificationCall(call tool.Call) (string, bool) {
	if call.Name != "bash" {
		return "", false
	}
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return "", false
	}
	return tool.VerificationCommand(input.Command)
}
