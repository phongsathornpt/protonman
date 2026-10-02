package runtime

import (
	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type HistoryCellKind = tuihistory.HistoryCellKind
type HistoryCell = tuihistory.HistoryCell
type HistoryState = tuihistory.HistoryState
type ScrollAnchor = tuihistory.ScrollAnchor
type UserCell = tuihistory.UserCell
type AssistantCell = tuihistory.AssistantCell
type AgentToolCell = tuihistory.AgentToolCell
type ToolCell = tuihistory.ToolCell
type ExecCell = tuihistory.ExecCell
type PatchCell = tuihistory.PatchCell
type AgentRunCell = tuihistory.AgentRunCell
type ReasoningCell = tuihistory.ReasoningCell
type SystemCell = tuihistory.SystemCell
type ErrorCell = tuihistory.ErrorCell

const (
	HistoryCellUnknown   = tuihistory.HistoryCellUnknown
	HistoryCellUser      = tuihistory.HistoryCellUser
	HistoryCellAssistant = tuihistory.HistoryCellAssistant
	HistoryCellReasoning = tuihistory.HistoryCellReasoning
	HistoryCellTool      = tuihistory.HistoryCellTool
	HistoryCellSystem    = tuihistory.HistoryCellSystem
	HistoryCellError     = tuihistory.HistoryCellError
)

func NewHistoryState(maxLines int) *HistoryState {
	return tuihistory.NewHistoryState(maxLines)
}

func summarizeToolOutput(name string, kind tool.Kind, target, body string, exitCode *int, truncated bool) string {
	return toolview.SummarizeOutput(name, kind, target, body, exitCode, truncated)
}

func shouldSuppressBody(kind tool.Kind, name string) bool {
	return toolview.ShouldSuppressBody(kind, name)
}
