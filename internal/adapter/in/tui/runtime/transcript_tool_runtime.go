package runtime

import (
	"context"
	"errors"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"strings"
	"time"
)

const maxBubbleScrollback = 1000

func (m *bubbleModel) ensureHistoryState() *HistoryState {
	if m.historyState == nil {
		m.historyState = NewHistoryState(maxBubbleScrollback)
	}
	return m.historyState
}

func (m *bubbleModel) appendLine(line string) {
	m.ensureHistoryState().Append(&SystemCell{Text: line})
}

func (m *bubbleModel) appendUser(line string) {
	m.ensureHistoryState().Append(&UserCell{Text: line})
}

func (m *bubbleModel) appendAssistant(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	m.ensureHistoryState().Append(&AssistantCell{Text: text})
}

func (m *bubbleModel) appendAssistantDelta(text string) {
	m.ensureHistoryState().AppendAssistantDelta(text)
}

func (m *bubbleModel) appendError(text string) {
	m.ensureHistoryState().Append(&ErrorCell{Text: text})
}

func (m *bubbleModel) appendMuted(text string) {
	m.ensureHistoryState().Append(&SystemCell{Text: text})
}

func (m *bubbleModel) appendToolRunning(name string) {
	m.ensureHistoryState().StartTool(name)
}

func toolFailureSuggestions(toolName string, code tool.ErrorCode) []string {
	var suggestions []string
	switch code {
	case tool.ErrorCodeNotFound:
		if strings.TrimSpace(toolName) == tool.NameRead {
			suggestions = append(suggestions, "ls the parent directory or find the filename")
		}
	case tool.ErrorCodeProtectedPath:
		suggestions = append(suggestions, "This path is shielded by workspace protection rules")
	case tool.ErrorCodeInternalPath:
		suggestions = append(suggestions, "Protonman internal state is reserved and unavailable to workspace tools")
	case tool.ErrorCodeOutsideWorkspace:
		suggestions = append(suggestions, "use . or a workspace-relative path")
	case tool.ErrorCodePermissionDenied:
		suggestions = append(suggestions, "Use shift+tab to cycle permission mode or allow the request")
	}
	return suggestions
}

func (m *bubbleModel) appendToolCall(call tool.Call) {
	state := m.ensureHistoryState()
	var kind tool.Kind
	if handler, ok := m.registry.Lookup(call.Name); ok {
		kind = handler.Definition().Kind
	}
	target, resolvedKind := extractToolTarget(call.Name, kind, call.Arguments)
	if target != "" {
		m.activity = "calling " + tool.DisplayName(call.Name) + " " + target
	} else {
		m.activity = "calling " + tool.DisplayName(call.Name)
	}
	if call.Name == "subagent" {
		action := extractStringArg(call.Arguments, "action")
		if action == "spawn" {
			m.rememberAgentRun(call)
			state.StartToolCell(&AgentToolCell{CallID: call.ID, Name: call.Name, Target: target, Running: true})
		} else {
			m.touchAgentOperation(call.Name, call)
		}
		return
	}
	switch resolvedKind {
	case tool.KindBash:
		cmd := target
		if cmd == "" {
			cmd = extractStringArg(call.Arguments, "command")
		}
		state.StartToolCell(&ExecCell{CallID: call.ID, Name: call.Name, Command: cmd, Running: true, StartedAt: time.Now()})
	case tool.KindEdit:
		if call.Name == "edit" && strings.EqualFold(extractStringArg(call.Arguments, "action"), "restore") {
			state.StartToolCell(&ToolCell{CallID: call.ID, Name: call.Name, Target: target, ToolKind: resolvedKind, Running: true})
			return
		}
		summary, paths := editPresentation(call)
		state.StartToolCell(&PatchCell{CallID: call.ID, Name: call.Name, Summary: summary, Paths: paths, Running: true})
	default:
		state.StartToolCell(&ToolCell{CallID: call.ID, Name: call.Name, Target: target, ToolKind: resolvedKind, Running: true})
	}
}

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
		body = joinBody(body, "checkpoint: "+result.CheckpointID)
	}
	if result.Failure == nil && err == nil && m.applyAgentToolResult(name, result, body) {
		return
	}
	if (result.Failure != nil || err != nil) && m.applyAgentToolFailure(name, result, err) {
		return
	}
	if result.Failure != nil && result.Failure.Message != "" && result.Failure.Code != tool.ErrorCodeCanceled {
		if name == "bash" && execFailureUsesExecCell(result.Failure.Code) {
			completed := m.completedToolCell(result.CallID, name, body, result)
			state.CompleteToolCall(result.CallID, name, completed)
			return
		}
		suggestions := toolFailureSuggestions(name, result.Failure.Code)
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
	if err != nil && !errors.Is(err, context.Canceled) && failureCode(result) != tool.ErrorCodeCanceled {
		errorCell := &ErrorCell{ErrorKind: ErrorKindToolFailed, Title: tool.DisplayName(name), Text: err.Error()}
		if result.Failure != nil {
			errorCell.Badge = string(result.Failure.Code)
			errorCell.Text = fmt.Sprintf("[%s]: %s", result.Failure.Code, result.Failure.Message)
			errorCell.Code = result.Failure.Code
		}
		state.CompleteToolCall(result.CallID, name, errorCell)
		return
	}
	if errors.Is(err, context.Canceled) || failureCode(result) == tool.ErrorCodeCanceled {
		body = "cancelled"
	}
	if name == "todo" && result.Failure == nil && err == nil && !result.Denied {
		state.DiscardToolCall(result.CallID, name)
		return
	}
	completed := m.completedToolCell(result.CallID, name, body, result)
	state.CompleteToolCall(result.CallID, name, completed)
}

func execFailureUsesExecCell(code tool.ErrorCode) bool {
	switch code {
	case tool.ErrorCodeCommandFailed, tool.ErrorCodeDeadlineExceeded:
		return true
	default:
		return false
	}
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

func failureCode(result tool.Result) tool.ErrorCode {
	if result.Failure == nil {
		return ""
	}
	return result.Failure.Code
}
