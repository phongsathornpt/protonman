package turn

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/projectTHORN/proton/internal/tool"
)

const (
	defaultMaxIdenticalNoProgressResults = 2
	defaultMaxIdenticalRetryableFailures = 3
)

type progressObservation struct {
	epoch       uint64
	resultHash  [sha256.Size]byte
	count       int
	failure     bool
	retryable   bool
	failureCode tool.ErrorCode
}

type progressGuard struct {
	maxIdenticalResults  int
	maxRetryableFailures int
	epoch                uint64
	observations         map[[sha256.Size]byte]progressObservation
	definitions          map[string]tool.Definition
}

func newProgressGuard(definitions []tool.Definition, maxIdenticalResults int) *progressGuard {
	byName := make(map[string]tool.Definition, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}
	return &progressGuard{
		maxIdenticalResults:  maxIdenticalResults,
		maxRetryableFailures: defaultMaxIdenticalRetryableFailures,
		observations:         make(map[[sha256.Size]byte]progressObservation),
		definitions:          byName,
	}
}

func (g *progressGuard) observeRound(executions []executedCall) (bool, error) {
	if g == nil || g.maxIdenticalResults <= 0 || len(executions) == 0 {
		return false, nil
	}

	allStalled := true
	tracked := false
	for _, execution := range executions {
		if execution.suppressed {
			tracked = true
			continue
		}
		stalled, isTracked, err := g.observe(execution)
		if err != nil {
			return false, err
		}
		if isTracked {
			tracked = true
		}
		if !stalled {
			allStalled = false
		}
	}
	return tracked && allStalled, nil
}

func (g *progressGuard) observe(execution executedCall) (stalled bool, tracked bool, err error) {
	definition, ok := g.definitions[execution.call.Name]
	if !ok {
		return false, false, nil
	}

	if execution.err == nil && potentiallyMutating(definition, execution.call) {
		g.epoch++
		return false, false, nil
	}
	if !shouldTrackNoProgress(definition, execution.result) {
		return false, false, nil
	}

	callHash, err := semanticCallHash(execution.call)
	if err != nil {
		return false, false, err
	}
	resultHash, err := noProgressResultHash(execution.result)
	if err != nil {
		return false, false, err
	}

	observation, exists := g.observations[callHash]
	if !exists || observation.epoch != g.epoch || observation.resultHash != resultHash {
		observation := progressObservation{
			epoch:      g.epoch,
			resultHash: resultHash,
			count:      1,
		}
		if execution.result.Failure != nil {
			observation.failure = true
			observation.retryable = execution.result.Failure.Retryable
			observation.failureCode = execution.result.Failure.Code
		}
		g.observations[callHash] = observation
		return false, true, nil
	}

	observation.count++
	g.observations[callHash] = observation
	limit := g.maxIdenticalResults
	if execution.result.Failure != nil && execution.result.Failure.Retryable {
		limit = g.maxRetryableFailures
	}
	return limit > 0 && observation.count >= limit, true, nil
}

func (g *progressGuard) suppress(call tool.Call) (*executedCall, error) {
	if g == nil || g.maxIdenticalResults <= 0 {
		return nil, nil
	}
	callHash, err := semanticCallHash(call)
	if err != nil {
		return nil, err
	}
	observation, ok := g.observations[callHash]
	if !ok || observation.epoch != g.epoch {
		return nil, nil
	}

	suppress := false
	switch {
	case observation.failure && !observation.retryable:
		suppress = observation.count >= 1
	case observation.failure && observation.retryable:
		suppress = g.maxRetryableFailures > 0 && observation.count >= g.maxRetryableFailures
	default:
		suppress = observation.count >= g.maxIdenticalResults
	}
	if !suppress {
		return nil, nil
	}

	code := tool.ErrorCodeNoProgress
	message := "identical tool call was suppressed after repeated no-progress results"
	denied := false
	if observation.failure && !observation.retryable {
		code = observation.failureCode
		message = "identical tool call was suppressed after a previous non-retryable failure"
		denied = code == tool.ErrorCodePermissionDenied
	}
	result := tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Denied:   denied,
		Failure: &tool.Failure{
			Code:    code,
			Message: message,
		},
	}
	return &executedCall{call: call, result: result, suppressed: true}, nil
}

func shouldTrackNoProgress(definition tool.Definition, result tool.Result) bool {
	if result.Failure != nil {
		return true
	}
	return definition.Kind == tool.KindRead || definition.Kind == tool.KindGrep
}

func potentiallyMutating(definition tool.Definition, call tool.Call) bool {
	return tool.EffectiveCallMutability(definition, call.Arguments) != tool.MutabilityReadOnly
}

func semanticCallHash(call tool.Call) ([sha256.Size]byte, error) {
	var arguments any
	if err := json.Unmarshal(call.Arguments, &arguments); err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("canonicalize %s arguments: %w", call.Name, err)
	}
	canonical, err := json.Marshal(arguments)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode canonical %s arguments: %w", call.Name, err)
	}
	payload := append([]byte(call.Name+"\x00"), canonical...)
	return sha256.Sum256(payload), nil
}

func noProgressResultHash(result tool.Result) ([sha256.Size]byte, error) {
	if result.Failure != nil {
		payload, err := json.Marshal(struct {
			Code      tool.ErrorCode `json:"code"`
			Retryable bool           `json:"retryable"`
		}{
			Code:      result.Failure.Code,
			Retryable: result.Failure.Retryable,
		})
		if err != nil {
			return [sha256.Size]byte{}, fmt.Errorf("encode semantic tool failure: %w", err)
		}
		return sha256.Sum256(payload), nil
	}
	return semanticResultHash(result)
}

func semanticResultHash(result tool.Result) ([sha256.Size]byte, error) {
	payload, err := json.Marshal(struct {
		ToolName     string        `json:"tool_name"`
		Output       string        `json:"output,omitempty"`
		ExitCode     *int          `json:"exit_code,omitempty"`
		Denied       bool          `json:"denied,omitempty"`
		Truncated    bool          `json:"truncated,omitempty"`
		NextOffset   *int64        `json:"next_offset,omitempty"`
		Failure      *tool.Failure `json:"error,omitempty"`
		CheckpointID string        `json:"checkpoint_id,omitempty"`
	}{
		ToolName:     result.ToolName,
		Output:       result.Output,
		ExitCode:     result.ExitCode,
		Denied:       result.Denied,
		Truncated:    result.Truncated,
		NextOffset:   result.NextOffset,
		Failure:      result.Failure,
		CheckpointID: result.CheckpointID,
	})
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode semantic tool result: %w", err)
	}
	return sha256.Sum256(payload), nil
}
