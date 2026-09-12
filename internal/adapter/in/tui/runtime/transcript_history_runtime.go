package runtime

import (
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/transcriptutil"
	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const maxBubbleScrollback = 1000

func (m *bubbleModel) ensureHistoryState() *tuihistory.HistoryState {
	if m.historyState == nil {
		m.historyState = tuihistory.NewHistoryState(maxBubbleScrollback)
	}
	return m.historyState
}

func (m *bubbleModel) appendLine(line string) {
	m.ensureHistoryState().Append(&tuihistory.SystemCell{Text: line})
}

func (m *bubbleModel) appendUser(line string) {
	m.ensureHistoryState().Append(&tuihistory.UserCell{Text: line})
}

func (m *bubbleModel) appendAssistant(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	m.ensureHistoryState().Append(&tuihistory.AssistantCell{Text: text})
}

func (m *bubbleModel) appendAssistantDelta(text string) {
	m.ensureHistoryState().AppendAssistantDelta(text)
}

func (m *bubbleModel) appendError(text string) {
	m.ensureHistoryState().Append(&tuihistory.ErrorCell{Text: text})
}

func (m *bubbleModel) appendMuted(text string) {
	m.ensureHistoryState().Append(&tuihistory.SystemCell{Text: text})
}

func (m *bubbleModel) appendToolRunning(name string) {
	m.ensureHistoryState().StartTool(name)
}

func (m *bubbleModel) appendToolCall(call tool.Call) {
	state := m.ensureHistoryState()
	var kind tool.Kind
	if handler, ok := m.registry.Lookup(call.Name); ok {
		kind = handler.Definition().Kind
	}
	target, resolvedKind := toolview.ExtractTarget(call.Name, kind, call.Arguments)
	if target != "" {
		m.activity = "calling " + tool.DisplayName(call.Name) + " " + target
	} else {
		m.activity = "calling " + tool.DisplayName(call.Name)
	}
	if call.Name == "subagent" {
		action := extractStringArg(call.Arguments, "action")
		if action == "spawn" {
			m.rememberAgentRun(call)
			state.StartToolCell(&tuihistory.AgentToolCell{CallID: call.ID, Name: call.Name, Target: target, Running: true})
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
		state.StartToolCell(&tuihistory.ExecCell{CallID: call.ID, Name: call.Name, Command: cmd, Running: true, StartedAt: time.Now()})
	case tool.KindEdit:
		if call.Name == "edit" && strings.EqualFold(extractStringArg(call.Arguments, "action"), "restore") {
			state.StartToolCell(&tuihistory.ToolCell{CallID: call.ID, Name: call.Name, Target: target, ToolKind: resolvedKind, Running: true})
			return
		}
		summary, paths := transcriptutil.EditPresentation(call)
		state.StartToolCell(&tuihistory.PatchCell{CallID: call.ID, Name: call.Name, Summary: summary, Paths: paths, Running: true})
	default:
		state.StartToolCell(&tuihistory.ToolCell{CallID: call.ID, Name: call.Name, Target: target, ToolKind: resolvedKind, Running: true})
	}
}
