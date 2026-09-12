package turn

import (
	"github.com/phongsathornpt/protonman/internal/base/strutil"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const toolBudgetMarker = "[tool output truncated by turn budget]"

type toolResultBudget struct {
	perRound int
	perTurn  int
	turnUsed int
}

func newToolResultBudget(perRound, perTurn int) *toolResultBudget {
	return &toolResultBudget{perRound: perRound, perTurn: perTurn}
}

func (b *toolResultBudget) applyRound(executions []executedCall) []executedCall {
	if b == nil {
		return executions
	}
	roundUsed := 0
	for i := range executions {
		size := toolResultTextBytes(executions[i].result)
		if size == 0 {
			continue
		}
		allowed := size
		if b.perRound > 0 {
			allowed = min(allowed, max(0, b.perRound-roundUsed))
		}
		if b.perTurn > 0 {
			allowed = min(allowed, max(0, b.perTurn-b.turnUsed))
		}
		if allowed < size {
			executions[i].result = truncateToolResultPayload(executions[i].result, allowed)
		}
		consumed := min(toolResultTextBytes(executions[i].result), allowed)
		roundUsed += consumed
		b.turnUsed += consumed
	}
	return executions
}

func toolResultTextBytes(result tool.Result) int {
	size := len(result.Output) + len(result.Stdout) + len(result.Stderr) + len(result.StructuredOutput)
	if result.Failure != nil && result.Failure.RecoveryEvidence != nil {
		size += len(result.Failure.RecoveryEvidence.Output) + len(result.Failure.RecoveryEvidence.StructuredOutput)
	}
	return size
}

func truncateToolResultPayload(result tool.Result, allowed int) tool.Result {
	if result.Failure != nil && result.Failure.RecoveryEvidence != nil &&
		len(result.Output)+len(result.Stdout)+len(result.Stderr)+len(result.StructuredOutput) == 0 {
		return truncateRecoveryEvidenceResult(result, allowed)
	}
	source := result.Output
	if source == "" {
		source = result.Stdout
		if result.Stderr != "" {
			if source != "" {
				source += "\n"
			}
			source += result.Stderr
		}
	}
	result.Stdout = ""
	result.Stderr = ""
	result.StdoutTruncated = result.StdoutTruncated || result.StdoutBytes > 0
	result.StderrTruncated = result.StderrTruncated || result.StderrBytes > 0
	result.Truncated = true

	if structuredBytes := len(result.StructuredOutput); structuredBytes > 0 {
		if structuredBytes > allowed {
			result.StructuredOutput = nil
			result.Output = truncateUTF8WithMarker("", allowed, toolBudgetMarker)
			if result.Failure == nil {
				result.Failure = &tool.Failure{
					Code:    tool.ErrorCodeOutputTooLarge,
					Message: "structured tool output exceeds the remaining turn result budget; request a smaller page or narrower result",
				}
			}
			return result
		}

		remaining := allowed - structuredBytes
		if len(source) > remaining {
			result.Output = truncateUTF8WithMarker(source, remaining, toolBudgetMarker)
		} else {
			result.Output = source
		}
		return result
	}

	result.StructuredOutput = nil
	result.Output = truncateUTF8WithMarker(source, allowed, toolBudgetMarker)
	return result
}

func truncateRecoveryEvidenceResult(result tool.Result, allowed int) tool.Result {
	failure := *result.Failure
	evidence := *failure.RecoveryEvidence
	structuredBytes := len(evidence.StructuredOutput)
	if structuredBytes > allowed {
		evidence.StructuredOutput = nil
		evidence.Output = truncateUTF8WithMarker("", allowed, toolBudgetMarker)
		evidence.SHA256 = ""
		evidence.Truncated = true
		failure.RecoveryEvidence = &evidence
		result.Failure = &failure
		result.Truncated = true
		return result
	}
	remaining := allowed - structuredBytes
	if len(evidence.Output) > remaining {
		evidence.Output = truncateUTF8WithMarker(evidence.Output, remaining, toolBudgetMarker)
		evidence.SHA256 = ""
		evidence.Truncated = true
		result.Truncated = true
	}
	failure.RecoveryEvidence = &evidence
	result.Failure = &failure
	return result
}

func truncateUTF8WithMarker(value string, limit int, marker string) string {
	return strutil.TruncateBytesWithMarker(value, limit, marker)
}
