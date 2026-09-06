package turn

import (
	"strings"
	"unicode/utf8"

	"github.com/projectTHORN/proton/internal/tool"
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
		consumed := min(size, allowed)
		if allowed < size {
			executions[i].result = truncateToolResultPayload(executions[i].result, allowed)
		}
		roundUsed += consumed
		b.turnUsed += consumed
	}
	return executions
}

func toolResultTextBytes(result tool.Result) int {
	return len(result.Output) + len(result.Stdout) + len(result.Stderr) + len(result.StructuredOutput)
}

func truncateToolResultPayload(result tool.Result, allowed int) tool.Result {
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
	if source == "" && len(result.StructuredOutput) > 0 {
		source = string(result.StructuredOutput)
	}
	result.Stdout = ""
	result.Stderr = ""
	result.StructuredOutput = nil
	result.StdoutTruncated = result.StdoutTruncated || result.StdoutBytes > 0
	result.StderrTruncated = result.StderrTruncated || result.StderrBytes > 0
	result.Truncated = true
	result.Output = truncateUTF8WithMarker(source, allowed, toolBudgetMarker)
	return result
}

func truncateUTF8WithMarker(value string, limit int, marker string) string {
	value = strings.ToValidUTF8(value, "�")
	if limit <= 0 {
		return marker
	}
	if len(marker) >= limit {
		return marker
	}
	prefixLimit := limit - len(marker) - 1
	if prefixLimit <= 0 {
		return marker
	}
	if len(value) > prefixLimit {
		value = value[:prefixLimit]
		for len(value) > 0 && !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return strings.TrimRight(value, "\n") + "\n" + marker
}
