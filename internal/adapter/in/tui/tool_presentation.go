package tui

import (
	"encoding/json"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func extractToolTarget(name string, kind tool.Kind, args json.RawMessage) (string, tool.Kind) {
	return toolview.ExtractTarget(name, kind, args)
}

func isAgentLifecycleTool(name string) bool            { return toolview.IsAgentLifecycleTool(name) }
func toolKindGlyph(kind tool.Kind, name string) string { return toolview.KindGlyph(kind, name) }
func summarizeToolOutput(name string, kind tool.Kind, target, body string, exitCode *int, truncated bool) string {
	return toolview.SummarizeOutput(name, kind, target, body, exitCode, truncated)
}
func shouldSuppressBody(kind tool.Kind, name string) bool {
	return toolview.ShouldSuppressBody(kind, name)
}
func formatOutputFold(lines []string, maxVisible int) []string {
	return toolview.FormatOutputFold(lines, maxVisible)
}
func styleDiffLine(line string) (string, bool) { return toolview.StyleDiffLine(line) }
func formatGrepToolView(lines []string, target string, width int) []string {
	return toolview.FormatGrepView(lines, target, width)
}
func formatPathSegmentsStyled(target string) string { return toolview.FormatPath(target) }

func extractSkillContentName(body string) string { return toolview.ExtractSkillContentName(body) }

func extractReadFileExcerpt(body string) string { return toolview.ExtractReadFileExcerpt(body) }
