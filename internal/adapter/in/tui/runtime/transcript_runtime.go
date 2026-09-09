package runtime

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
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

func (m *bubbleModel) applyTurnEvents(events []app.Event) {
	if len(events) == 0 {
		return
	}
	var pending strings.Builder
	pendingRound := 0
	flushText := func() {
		if pending.Len() == 0 {
			return
		}
		m.applyTurnEvent(app.Event{Kind: app.EventTextDelta, Round: pendingRound, Text: pending.String()})
		pending.Reset()
		pendingRound = 0
	}
	for _, event := range events {
		if event.Kind == app.EventTextDelta {
			pending.WriteString(event.Text)
			if event.Round > pendingRound {
				pendingRound = event.Round
			}
			continue
		}
		flushText()
		m.applyTurnEvent(event)
	}
	flushText()
}

func (m *bubbleModel) applyTurnEvent(event app.Event) {
	if event.Round > 0 {
		m.turnProgress.Round = event.Round
	}
	switch event.Kind {
	case app.EventTextDelta:
		m.activity = "synthesizing"
		m.appendAssistantDelta(event.Text)
	case app.EventToolCall:
		m.turnProgress.ToolCalls++
		m.appendToolCall(event.Call)
	case app.EventToolResult:
		result := event.Result
		if result.CallID == "" {
			result.CallID = event.Call.ID
		}
		if result.ToolName == "" {
			result.ToolName = event.Call.Name
		}
		m.applyToolResult(event.Call.Name, result, event.Err)
		m.syncTodoSnapshot()
		m.activity = "analyzing"
	case app.EventCompleted:
		m.ensureHistoryState().CommitActive()
	case app.EventFailed:
		m.appendTurnFailure(event.Err)
	}
}

func (m *bubbleModel) appendTurnFailure(err error) {
	if err == nil {
		return
	}
	classified := ClassifyOpenCodeError(err, m.activeProvider, m.activeModel)
	if classified.Kind == ErrorKindCancelled {
		text := "turn cancelled"
		cells := m.ensureHistoryState().Cells()
		if n := len(cells); n > 0 {
			if last, ok := cells[n-1].(*SystemCell); ok && last.Text == text {
				return
			}
		}
		m.ensureHistoryState().Append(&SystemCell{Text: text})
		return
	}
	cells := m.ensureHistoryState().Cells()
	if n := len(cells); n > 0 {
		if last, ok := cells[n-1].(*ErrorCell); ok && last.Text == classified.Message && last.Title == classified.Title {
			return
		}
	}
	m.ensureHistoryState().Append(&ErrorCell{ErrorKind: classified.Kind, Title: classified.Title, Badge: classified.Badge, Text: classified.Message, Suggestions: classified.Suggestions, RawDetails: classified.RawDetails, Retryable: classified.Retryable})
}

func (m *bubbleModel) appendTurnResult(events []app.Event, result app.Result, err error) {
	sawAssistant := false
	for _, event := range events {
		if event.Kind == app.EventTextDelta && event.Text != "" {
			sawAssistant = true
		}
		m.applyTurnEvent(event)
	}
	if !sawAssistant && result.Message.Content != "" {
		m.appendAssistant(result.Message.Content)
	}
	m.ensureHistoryState().CommitActive()
	m.appendTurnFailure(err)
}

func (m *bubbleModel) appendToolResult(result tool.Result, err error) {
	m.applyToolResult(result.ToolName, result, err)
	if err == nil {
		m.appendMuted("tool completed")
	}
}

func (m bubbleModel) renderBlocks() []string {
	if m.historyState == nil {
		return nil
	}
	return m.historyState.RenderLines()
}

func joinBody(existing string, extra string) string {
	if existing == "" {
		return extra
	}
	if extra == "" {
		return existing
	}
	return existing + "\n" + extra
}

func plainTranscript(model *bubbleModel) string {
	if model == nil || model.historyState == nil {
		return ""
	}
	return model.historyState.Raw()
}

func (m *bubbleModel) loadInitialMessages(messages []model.Message) {
	state := m.ensureHistoryState()
	for _, message := range messages {
		text := strings.TrimSpace(message.Content)
		switch message.Role {
		case model.RoleUser:
			if text != "" {
				if strings.HasPrefix(text, "Activated skill ") {
					if idx := strings.Index(text, "\n"); idx != -1 {
						text = text[:idx]
					}
				}
				state.Append(&UserCell{Text: text})
			}
		case model.RoleAssistant:
			if text != "" {
				state.Append(&AssistantCell{Text: message.Content})
			}
		case model.RoleTool:
			if text != "" || message.ToolName != "" {
				kind := tool.KindForName(message.ToolName)
				summary := summarizeToolOutput(message.ToolName, kind, "", message.Content, nil, false)
				state.Append(&ToolCell{Name: message.ToolName, Body: message.Content, ToolKind: kind, Summary: summary})
			}
		case model.RoleSystem:
			if text != "" {
				state.Append(&SystemCell{Text: message.Content})
			}
		}
	}
}

func extractStringArg(raw json.RawMessage, key string) string {
	call := tool.Call{Arguments: raw}
	return tool.ExtractString(call.ArgumentsMap(), key)
}

func editPresentation(call tool.Call) (string, []string) {
	paths := call.AffectedPaths()
	summary := "editing workspace"
	if len(paths) == 1 {
		summary = "1 file"
	} else if len(paths) > 1 {
		summary = fmt.Sprintf("%d files", len(paths))
	}
	return summary, paths
}

func (m *bubbleModel) closeTranscriptOverlay() {
	if m == nil {
		return
	}
	m.showTranscript = false
	if m.historyState != nil {
		m.historyState.ReleaseAlternateRenderCache()
		m.historyState.ReleaseRawTextCache()
	}
	m.transcriptViewport.SetContent("")
}

func (m *bubbleModel) refreshTranscriptViewport(forceTail bool) {
	if m.historyState == nil {
		return
	}
	follow := forceTail || m.transcriptViewport.AtBottom()
	content := m.historyState.Raw()
	if !m.rawTranscript {
		content = strings.Join(m.historyState.RenderLinesAt(maxInt(8, m.transcriptViewport.Width())), "\n")
	}
	if strings.TrimSpace(content) == "" {
		content = mutedStyle.Render("No transcript yet.")
	}
	m.transcriptViewport.SetContent(content)
	if follow {
		m.transcriptViewport.GotoBottom()
	}
}

func (m *bubbleModel) transcriptOverlayView() string {
	mode := "rich"
	if m.rawTranscript {
		mode = "raw"
	}
	header := brandStyle.Render("Transcript") + mutedStyle.Render(" · "+mode)
	footer := mutedStyle.Render("esc/ctrl+t close · r raw/rich · pgup/pgdn scroll")
	body := lipgloss.JoinVertical(lipgloss.Left, header, m.transcriptViewport.View(), footer)
	width := maxInt(1, m.width-6)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentAssistant).Padding(0, 1).Width(width).Render(body)
}

func (m *bubbleModel) updateTranscriptKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(message, m.keys.Transcript) {
		m.closeTranscriptOverlay()
		return m, nil
	}
	switch message.String() {
	case "esc", "q":
		m.closeTranscriptOverlay()
		return m, nil
	case "r":
		m.rawTranscript = !m.rawTranscript
		if m.historyState != nil {
			if m.rawTranscript {
				m.historyState.ReleaseAlternateRenderCache()
			} else {
				m.historyState.ReleaseRawTextCache()
			}
		}
		m.refreshTranscriptViewport(false)
		return m, nil
	case "pgup":
		m.transcriptViewport.PageUp()
		return m, nil
	case "pgdown":
		m.transcriptViewport.PageDown()
		return m, nil
	case "up", "k":
		m.transcriptViewport.ScrollUp(1)
		return m, nil
	case "down", "j":
		m.transcriptViewport.ScrollDown(1)
		return m, nil
	case "home", "g":
		m.transcriptViewport.GotoTop()
		return m, nil
	case "end", "G":
		m.transcriptViewport.GotoBottom()
		return m, nil
	default:
		return m, nil
	}
}
