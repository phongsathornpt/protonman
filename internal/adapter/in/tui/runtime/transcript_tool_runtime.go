package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/transcriptutil"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func (m *bubbleModel) applyToolResult(name string, result tool.Result, err error) {
	if name == "" {
		name = result.ToolName
	}
	if name == "bash" {
		// Shell commands may change HEAD or switch worktrees. Refresh the cached
		// welcome metadata once after completion instead of reading .git during View.
		m.invalidateWelcomeBranch()
	}
	if name == "" {
		name = m.lastRunningToolName()
	}
	state := m.ensureHistoryState()
	body := result.Output
	name = strings.TrimSpace(name)
	if len(result.StructuredOutput) > 0 && (name == "todo" || name == "subagent") {
		body = string(result.StructuredOutput)
	}
	if result.CheckpointID != "" {
		body = transcriptutil.JoinBody(body, "checkpoint: "+result.CheckpointID)
	}
	if result.Failure == nil && err == nil && m.applyAgentToolResult(name, result, body) {
		return
	}
	if (result.Failure != nil || err != nil) && m.applyAgentToolFailure(name, result, err) {
		return
	}
	if result.Failure != nil && result.Failure.Message != "" && result.Failure.Code != tool.ErrorCodeCanceled {
		if name == "bash" && transcriptutil.ExecFailureUsesExecCell(result.Failure.Code) {
			completed := m.completedToolCell(result.CallID, name, body, result)
			state.CompleteToolCall(result.CallID, name, completed)
			return
		}
		suggestions := transcriptutil.ToolFailureSuggestions(name, result.Failure.Code)
		title := tool.DisplayName(name)
		badge := string(result.Failure.Code)
		text := result.Failure.Message
		if name == "todo" && result.Failure.Code == tool.ErrorCodeConflict {
			title = "Task plan changed"
			badge = "stale"
			text = "The task plan changed while this update was being prepared."
			suggestions = []string{"Refresh tasks with todo action=get, then retry the update."}
		}
		errorCell := &ErrorCell{ErrorKind: ErrorKindToolFailed, Title: title, Badge: badge, Text: text, Code: result.Failure.Code, Suggestions: suggestions}
		state.CompleteToolCall(result.CallID, name, errorCell)
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) && transcriptutil.FailureCode(result) != tool.ErrorCodeCanceled {
		errorCell := &ErrorCell{ErrorKind: ErrorKindToolFailed, Title: tool.DisplayName(name), Text: err.Error()}
		if result.Failure != nil {
			errorCell.Badge = string(result.Failure.Code)
			errorCell.Text = fmt.Sprintf("[%s]: %s", result.Failure.Code, result.Failure.Message)
			errorCell.Code = result.Failure.Code
		}
		state.CompleteToolCall(result.CallID, name, errorCell)
		return
	}
	if errors.Is(err, context.Canceled) || transcriptutil.FailureCode(result) == tool.ErrorCodeCanceled {
		body = "cancelled"
	}
	if name == "todo" && result.Failure == nil && err == nil && !result.Denied {
		state.DiscardToolCall(result.CallID, name)
		return
	}
	completed := m.completedToolCell(result.CallID, name, body, result)
	state.CompleteToolCall(result.CallID, name, completed)
}

func (m *bubbleModel) finalizeRunningTools(err error) {
	if err == nil {
		return
	}
	failure := tool.FailureFromError(err)
	if failure == nil {
		failure = &tool.Failure{Code: tool.ErrorCodeExecution, Message: err.Error()}
	}
	body := "aborted"
	switch failure.Code {
	case tool.ErrorCodeCanceled:
		body = "cancelled"
	case tool.ErrorCodeDeadlineExceeded:
		body = "timed out"
	}
	for _, running := range m.ensureHistoryState().RunningTools() {
		m.applyToolResult(running.Name, tool.Result{CallID: running.CallID, ToolName: running.Name, Output: body, Failure: failure}, nil)
	}
}

func (m *bubbleModel) completedToolCell(callID string, name string, body string, result tool.Result) HistoryCell {
	var failureCode tool.ErrorCode
	if result.Failure != nil {
		failureCode = result.Failure.Code
	}
	var target string
	var toolKind tool.Kind
	if running := m.runningToolCell(callID, name); running != nil {
		switch typed := running.(type) {
		case *ExecCell:
			duration := time.Duration(0)
			if !typed.StartedAt.IsZero() {
				duration = time.Since(typed.StartedAt)
			}
			return &ExecCell{CallID: typed.CallID, Name: typed.Name, Command: typed.Command, StartedAt: typed.StartedAt, Duration: duration, Body: body, Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode, Truncated: result.Truncated, StdoutTruncated: result.StdoutTruncated, StderrTruncated: result.StderrTruncated, Denied: result.Denied, FailureCode: failureCode}
		case *PatchCell:
			return &PatchCell{CallID: typed.CallID, Name: typed.Name, Summary: typed.Summary, Paths: append([]string{}, typed.Paths...), Body: body, Truncated: result.Truncated, Denied: result.Denied, FailureCode: failureCode}
		case *AgentToolCell:
			return &AgentToolCell{CallID: typed.CallID, Name: typed.Name, Target: typed.Target, Summary: summarizeToolOutput(typed.Name, tool.KindAgent, typed.Target, body, result.ExitCode, result.Truncated)}
		case *ToolCell:
			callID = typed.CallID
			name = typed.Name
			target = typed.Target
			toolKind = typed.ToolKind
		}
	}
	if toolKind == "" {
		if handler, ok := m.registry.Lookup(name); ok {
			toolKind = handler.Definition().Kind
		} else {
			toolKind = tool.KindForName(name)
		}
	}
	summary := summarizeToolOutput(name, toolKind, target, body, result.ExitCode, result.Truncated)
	return &ToolCell{CallID: callID, Name: name, Body: body, Target: target, ToolKind: toolKind, Summary: summary, ExitCode: result.ExitCode, Truncated: result.Truncated, Denied: result.Denied, FailureCode: failureCode}
}

func (m *bubbleModel) runningToolCell(callID string, name string) HistoryCell {
	return m.ensureHistoryState().FindRunningTool(callID, name)
}

func (m *bubbleModel) lastRunningToolName() string {
	return m.ensureHistoryState().LastRunningToolName()
}
