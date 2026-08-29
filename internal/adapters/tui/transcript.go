package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const maxBubbleScrollback = 1000

// Block remains as a derived compatibility snapshot for the existing package
// tests while the TUI migrates to HistoryCell. Runtime rendering no longer uses
// Block as its source of truth.
type blockKind uint8

const (
	blockUser blockKind = iota
	blockAssistant
	blockTool
	blockSystem
	blockError
)

type Block struct {
	Kind    blockKind
	Title   string
	Body    string
	Running bool
	Code    string
}

func (m *bubbleModel) ensureHistoryState() *HistoryState {
	if m.historyState == nil {
		m.historyState = NewHistoryState(maxBubbleScrollback)
	}
	return m.historyState
}

func (m *bubbleModel) syncLegacyBlocks() {
	cells := m.ensureHistoryState().Cells()
	blocks := make([]Block, 0, len(cells))
	for _, cell := range cells {
		switch typed := cell.(type) {
		case *UserCell:
			blocks = append(blocks, Block{Kind: blockUser, Body: typed.Text})
		case *AssistantCell:
			blocks = append(blocks, Block{Kind: blockAssistant, Body: typed.Text})
		case *ToolCell:
			blocks = append(blocks, Block{Kind: blockTool, Title: typed.Name, Body: typed.Body, Running: typed.Running, Code: typed.FailureCode})
		case *ExecCell:
			blocks = append(blocks, Block{Kind: blockTool, Title: typed.Name, Body: typed.Body, Running: typed.Running, Code: typed.FailureCode})
		case *PatchCell:
			blocks = append(blocks, Block{Kind: blockTool, Title: typed.Name, Body: typed.Body, Running: typed.Running, Code: typed.FailureCode})
		case *SystemCell:
			blocks = append(blocks, Block{Kind: blockSystem, Body: typed.Text})
		case *ErrorCell:
			blocks = append(blocks, Block{Kind: blockError, Title: typed.Title, Body: typed.Text, Code: typed.Code})
		}
	}
	m.blocks = blocks
}

func (m *bubbleModel) pushBlock(block Block) {
	state := m.ensureHistoryState()
	switch block.Kind {
	case blockUser:
		state.Append(&UserCell{Text: block.Body})
	case blockAssistant:
		state.Append(&AssistantCell{Text: block.Body})
	case blockTool:
		cell := &ToolCell{Name: block.Title, Body: block.Body, Running: block.Running, FailureCode: block.Code}
		if block.Running {
			state.StartToolCell(cell)
		} else {
			state.Append(cell)
		}
	case blockError:
		state.Append(&ErrorCell{Title: block.Title, Text: block.Body, Code: block.Code})
	default:
		state.Append(&SystemCell{Text: block.Body})
	}
	m.syncLegacyBlocks()
}

func (m *bubbleModel) trimBlocks() { m.syncLegacyBlocks() }

func lineCount(blocks []Block) int {
	total := 0
	for _, block := range blocks {
		total += 1 + strings.Count(block.Body, "\n")
		if block.Title != "" && block.Kind == blockTool {
			total++
		}
	}
	return total
}

func (m *bubbleModel) appendLine(line string) {
	m.ensureHistoryState().Append(&SystemCell{Text: line})
	m.syncLegacyBlocks()
}

func (m *bubbleModel) appendUser(line string) {
	m.ensureHistoryState().Append(&UserCell{Text: line})
	m.syncLegacyBlocks()
}

func (m *bubbleModel) appendAssistant(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	m.ensureHistoryState().Append(&AssistantCell{Text: text})
	m.syncLegacyBlocks()
}

func (m *bubbleModel) appendAssistantDelta(text string) {
	m.ensureHistoryState().AppendAssistantDelta(text)
	m.syncLegacyBlocks()
}

func (m *bubbleModel) appendError(text string) {
	m.ensureHistoryState().Append(&ErrorCell{Text: text})
	m.syncLegacyBlocks()
}

func (m *bubbleModel) appendMuted(text string) {
	m.ensureHistoryState().Append(&SystemCell{Text: text})
	m.syncLegacyBlocks()
}

func (m *bubbleModel) appendToolRunning(name string) {
	m.ensureHistoryState().StartTool(name)
	m.syncLegacyBlocks()
}

func (m *bubbleModel) appendToolCall(call tool.Call) {
	state := m.ensureHistoryState()
	handler, ok := m.registry.Lookup(call.Name)
	if !ok {
		state.StartToolCall(call.ID, call.Name)
		m.syncLegacyBlocks()
		return
	}
	switch handler.Definition().Kind {
	case tool.KindBash:
		state.StartToolCell(&ExecCell{
			CallID:  call.ID,
			Name:    call.Name,
			Command: extractStringArg(call.Arguments, "command"),
			Running: true,
		})
	case tool.KindEdit:
		summary, paths := editPresentation(call)
		state.StartToolCell(&PatchCell{
			CallID:  call.ID,
			Name:    call.Name,
			Summary: summary,
			Paths:   paths,
			Running: true,
		})
	default:
		state.StartToolCall(call.ID, call.Name)
	}
	m.syncLegacyBlocks()
}

func (m *bubbleModel) applyToolResult(name string, result tool.Result, err error) {
	if name == "" {
		name = result.ToolName
	}
	if name == "" {
		name = m.lastRunningToolName()
	}
	state := m.ensureHistoryState()
	body := result.Output
	if result.CheckpointID != "" {
		body = joinBody(body, "checkpoint: "+result.CheckpointID)
	}

	if err != nil && !errors.Is(err, context.Canceled) && failureCode(result) != tool.ErrorCodeCanceled {
		errorCell := &ErrorCell{Title: name, Text: err.Error()}
		if result.Failure != nil {
			errorCell.Text = fmt.Sprintf("[%s]: %s", result.Failure.Code, result.Failure.Message)
			errorCell.Code = string(result.Failure.Code)
		}
		state.CompleteToolCall(result.CallID, name, errorCell)
		m.syncLegacyBlocks()
		return
	}

	if errors.Is(err, context.Canceled) || failureCode(result) == tool.ErrorCodeCanceled {
		body = "cancelled"
	}
	completed := m.completedToolCell(result.CallID, name, body, result)
	state.CompleteToolCall(result.CallID, name, completed)
	m.syncLegacyBlocks()
}

func (m *bubbleModel) completedToolCell(callID string, name string, body string, result tool.Result) HistoryCell {
	failureCode := ""
	if result.Failure != nil {
		failureCode = string(result.Failure.Code)
	}
	if running := m.runningToolCell(callID, name); running != nil {
		switch typed := running.(type) {
		case *ExecCell:
			return &ExecCell{
				CallID:      typed.CallID,
				Name:        typed.Name,
				Command:     typed.Command,
				Body:        body,
				ExitCode:    result.ExitCode,
				Truncated:   result.Truncated,
				Denied:      result.Denied,
				FailureCode: failureCode,
			}
		case *PatchCell:
			return &PatchCell{
				CallID:      typed.CallID,
				Name:        typed.Name,
				Summary:     typed.Summary,
				Paths:       append([]string{}, typed.Paths...),
				Body:        body,
				Truncated:   result.Truncated,
				Denied:      result.Denied,
				FailureCode: failureCode,
			}
		case *ToolCell:
			callID = typed.CallID
			name = typed.Name
		}
	}
	return &ToolCell{
		CallID:      callID,
		Name:        name,
		Body:        body,
		ExitCode:    result.ExitCode,
		Truncated:   result.Truncated,
		Denied:      result.Denied,
		FailureCode: failureCode,
	}
}

func (m *bubbleModel) runningToolCell(callID string, name string) HistoryCell {
	state := m.ensureHistoryState()
	if runningToolMatches(state.Active(), callID, name) {
		return state.Active()
	}
	committed := state.Committed()
	for i := len(committed) - 1; i >= 0; i-- {
		if runningToolMatches(committed[i], callID, name) {
			return committed[i]
		}
	}
	return nil
}

func (m *bubbleModel) lastRunningToolName() string {
	cells := m.ensureHistoryState().Cells()
	for i := len(cells) - 1; i >= 0; i-- {
		if running, ok := cells[i].(runningHistoryTool); ok && running.historyToolRunning() {
			return running.historyToolName()
		}
	}
	return ""
}

func failureCode(result tool.Result) tool.ErrorCode {
	if result.Failure == nil {
		return ""
	}
	return result.Failure.Code
}

func (m *bubbleModel) replaceRunningTool(name string, replacement Block) bool {
	if m.runningToolCell("", name) == nil {
		return false
	}
	var cell HistoryCell
	switch replacement.Kind {
	case blockError:
		cell = &ErrorCell{Title: replacement.Title, Text: replacement.Body, Code: replacement.Code}
	default:
		cell = &ToolCell{Name: replacement.Title, Body: replacement.Body, Running: replacement.Running, FailureCode: replacement.Code}
	}
	m.ensureHistoryState().CompleteToolCell(name, cell)
	m.syncLegacyBlocks()
	return true
}

func (m *bubbleModel) applyTurnEvent(event applicationturn.Event) {
	switch event.Kind {
	case applicationturn.EventTextDelta:
		m.appendAssistantDelta(event.Text)
	case applicationturn.EventToolCall:
		m.appendToolCall(event.Call)
	case applicationturn.EventToolResult:
		result := event.Result
		if result.CallID == "" {
			result.CallID = event.Call.ID
		}
		if result.ToolName == "" {
			result.ToolName = event.Call.Name
		}
		m.applyToolResult(event.Call.Name, result, nil)
	case applicationturn.EventCompleted:
		m.ensureHistoryState().CommitActive()
		m.syncLegacyBlocks()
	case applicationturn.EventFailed:
		m.appendTurnFailure(event.Err)
	}
}

func (m *bubbleModel) appendTurnFailure(err error) {
	if err == nil {
		return
	}
	text := "turn failed: " + err.Error()
	if errors.Is(err, context.Canceled) {
		text = "turn cancelled"
		cells := m.ensureHistoryState().Cells()
		if n := len(cells); n > 0 {
			if last, ok := cells[n-1].(*SystemCell); ok && last.Text == text {
				return
			}
		}
		m.ensureHistoryState().Append(&SystemCell{Text: text})
		m.syncLegacyBlocks()
		return
	}
	cells := m.ensureHistoryState().Cells()
	if n := len(cells); n > 0 {
		if last, ok := cells[n-1].(*ErrorCell); ok && last.Text == text {
			return
		}
	}
	m.ensureHistoryState().Append(&ErrorCell{Text: text})
	m.syncLegacyBlocks()
}

func (m *bubbleModel) appendTurnResult(events []applicationturn.Event, result applicationturn.Result, err error) {
	sawAssistant := false
	for _, event := range events {
		if event.Kind == applicationturn.EventTextDelta && event.Text != "" {
			sawAssistant = true
		}
		m.applyTurnEvent(event)
	}
	if !sawAssistant && result.Message.Content != "" {
		m.appendAssistant(result.Message.Content)
	}
	m.ensureHistoryState().CommitActive()
	m.syncLegacyBlocks()
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

func renderBlock(block Block) []string {
	switch block.Kind {
	case blockUser:
		return (&UserCell{Text: block.Body}).Render()
	case blockAssistant:
		return (&AssistantCell{Text: block.Body}).Render()
	case blockTool:
		return (&ToolCell{Name: block.Title, Body: block.Body, Running: block.Running, FailureCode: block.Code}).Render()
	case blockError:
		return (&ErrorCell{Title: block.Title, Text: block.Body, Code: block.Code}).Render()
	default:
		return (&SystemCell{Text: block.Body}).Render()
	}
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
				state.Append(&UserCell{Text: message.Content})
			}
		case model.RoleAssistant:
			if text != "" {
				state.Append(&AssistantCell{Text: message.Content})
			}
		case model.RoleTool:
			if text != "" || message.ToolName != "" {
				state.Append(&ToolCell{Name: message.ToolName, Body: message.Content})
			}
		case model.RoleSystem:
			if text != "" {
				state.Append(&SystemCell{Text: message.Content})
			}
		}
	}
	m.syncLegacyBlocks()
}

func extractStringArg(raw json.RawMessage, key string) string {
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func editPresentation(call tool.Call) (string, []string) {
	paths := make([]string, 0, 4)
	var values map[string]any
	if json.Unmarshal(call.Arguments, &values) == nil {
		for _, key := range []string{"path", "file", "filename", "target", "destination", "move_path"} {
			if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
				paths = appendUnique(paths, strings.TrimSpace(value))
			}
		}
		for _, key := range []string{"patch", "input", "diff"} {
			if value, ok := values[key].(string); ok {
				for _, path := range patchPaths(value) {
					paths = appendUnique(paths, path)
				}
			}
		}
	}
	summary := "editing workspace"
	if len(paths) == 1 {
		summary = "1 file"
	} else if len(paths) > 1 {
		summary = fmt.Sprintf("%d files", len(paths))
	}
	return summary, paths
}

func patchPaths(patch string) []string {
	paths := make([]string, 0)
	for _, line := range strings.Split(patch, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, prefix := range []string{"*** Add File:", "*** Update File:", "*** Delete File:", "*** Move to:"} {
			if strings.HasPrefix(trimmed, prefix) {
				path := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
				if path != "" {
					paths = appendUnique(paths, path)
				}
			}
		}
	}
	return paths
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
