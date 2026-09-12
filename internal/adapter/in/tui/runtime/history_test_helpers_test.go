package runtime

import (
	"encoding/json"

	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	tuipresentation "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/presentation"
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
type SystemCell = tuihistory.SystemCell
type ErrorCell = tuihistory.ErrorCell

const (
	HistoryCellUnknown   = tuihistory.HistoryCellUnknown
	HistoryCellUser      = tuihistory.HistoryCellUser
	HistoryCellAssistant = tuihistory.HistoryCellAssistant
	HistoryCellTool      = tuihistory.HistoryCellTool
	HistoryCellSystem    = tuihistory.HistoryCellSystem
	HistoryCellError     = tuihistory.HistoryCellError
)

func NewHistoryState(maxLines int) *HistoryState {
	return tuihistory.NewHistoryState(maxLines)
}

func extractToolTarget(name string, kind tool.Kind, args json.RawMessage) (string, tool.Kind) {
	return toolview.ExtractTarget(name, kind, args)
}

func toolKindGlyph(kind tool.Kind, name string) string {
	return toolview.KindGlyph(kind, name)
}

func summarizeToolOutput(name string, kind tool.Kind, target, body string, exitCode *int, truncated bool) string {
	return toolview.SummarizeOutput(name, kind, target, body, exitCode, truncated)
}

func shouldSuppressBody(kind tool.Kind, name string) bool {
	return toolview.ShouldSuppressBody(kind, name)
}

func minimalToolShowsDetail(kind tool.Kind, denied bool, failed bool) bool {
	return tuipresentation.MinimalPolicy().ToolDetail(kind, denied, failed) != tuipresentation.DetailSummary
}

func formatOutputFold(lines []string, maxVisible int) []string {
	return toolview.FormatOutputFold(lines, maxVisible)
}

func styleDiffLine(line string) (string, bool) {
	return toolview.StyleDiffLine(line)
}

func formatGrepToolView(lines []string, target string, width int) []string {
	return toolview.FormatGrepView(lines, target, width)
}

func formatPathSegmentsStyled(target string) string {
	return toolview.FormatPath(target)
}

func extractSkillContentName(body string) string {
	return toolview.ExtractSkillContentName(body)
}

func extractReadFileExcerpt(body string) string {
	return toolview.ExtractReadFileExcerpt(body)
}
