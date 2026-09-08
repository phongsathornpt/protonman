package tui

import tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/history"

type HistoryCellKind = tuihistory.HistoryCellKind

type HistoryCell = tuihistory.HistoryCell

type HistoryState = tuihistory.HistoryState

type UserCell = tuihistory.UserCell
type AssistantCell = tuihistory.AssistantCell
type AgentToolCell = tuihistory.AgentToolCell
type ToolCell = tuihistory.ToolCell
type ExecCell = tuihistory.ExecCell
type PatchCell = tuihistory.PatchCell
type AgentRunCell = tuihistory.AgentRunCell
type SystemCell = tuihistory.SystemCell
type ErrorCell = tuihistory.ErrorCell
type ThinkingCell = tuihistory.ThinkingCell

const (
	HistoryCellUnknown   = tuihistory.HistoryCellUnknown
	HistoryCellUser      = tuihistory.HistoryCellUser
	HistoryCellAssistant = tuihistory.HistoryCellAssistant
	HistoryCellTool      = tuihistory.HistoryCellTool
	HistoryCellSystem    = tuihistory.HistoryCellSystem
	HistoryCellError     = tuihistory.HistoryCellError
)

func NewHistoryState(maxLines int) *HistoryState { return tuihistory.NewHistoryState(maxLines) }
