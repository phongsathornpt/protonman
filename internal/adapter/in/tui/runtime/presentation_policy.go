package runtime

import (
	"image/color"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/diagnostic"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
)

type OpenCodeErrorKind = diagnostic.Kind

type ClassifiedError = diagnostic.Error

const (
	ErrorKindModelNotFound    = diagnostic.Kind("model_not_found")
	ErrorKindContextOverflow  = diagnostic.Kind("context_overflow")
	ErrorKindAuthentication   = diagnostic.Kind("authentication")
	ErrorKindForbidden        = diagnostic.Kind("forbidden")
	ErrorKindRateLimit        = diagnostic.Kind("rate_limit")
	ErrorKindQuotaExceeded    = diagnostic.Kind("quota_exceeded")
	ErrorKindServerOverloaded = diagnostic.Kind("server_overloaded")
	ErrorKindStreamTimeout    = diagnostic.Kind("stream_timeout")
	ErrorKindStreamIncomplete = diagnostic.Kind("stream_incomplete")
	ErrorKindEmptyResponse    = diagnostic.Kind("empty_response")
	ErrorKindInvalidPrompt    = diagnostic.Kind("invalid_prompt")
	ErrorKindMCPFailed        = diagnostic.Kind("mcp_failed")
	ErrorKindConfigInvalid    = diagnostic.Kind("config_invalid")
	ErrorKindConfigTypo       = diagnostic.Kind("config_typo")
	ErrorKindToolFailed       = diagnostic.Kind("tool_failed")
	ErrorKindToolDispatch     = diagnostic.Kind("tool_dispatch")
	ErrorKindPermissionDenied = diagnostic.Kind("permission_denied")
	ErrorKindCancelled        = diagnostic.Kind("cancelled")
	ErrorKindGeneric          = diagnostic.Kind("generic")
)

func ClassifyOpenCodeError(err error, activeProvider, activeModel string) ClassifiedError {
	return diagnostic.Classify(err, activeProvider, activeModel)
}

func FormatErrorSummary(classified ClassifiedError) string {
	return diagnostic.FormatSummary(classified)
}

type terminalLayoutMode = panecommon.LayoutMode

const (
	layoutNormal  = panecommon.LayoutNormal
	layoutCompact = panecommon.LayoutCompact
	layoutTiny    = panecommon.LayoutTiny
)

func layoutModeForHeight(height int) terminalLayoutMode {
	return panecommon.ModeForHeight(height)
}

func renderModalRows(ctx paneRenderContext, border color.Color, rows []string) string {
	return panecommon.RenderModal(ctx.width, ctx.height, border, rows)
}

func paneWindow(count, selected, maximum int, mode terminalLayoutMode) (int, int) {
	visible := maximum
	switch mode {
	case layoutTiny:
		visible = minInt(2, maximum)
	case layoutCompact:
		visible = minInt(4, maximum)
	}
	if visible < 1 {
		visible = 1
	}
	if count <= visible {
		return 0, count
	}
	start := selected - visible + 1
	if start < 0 {
		start = 0
	}
	if start+visible > count {
		start = count - visible
	}
	return start, start + visible
}

func paneKeyboardHelp(width int, bindings ...string) string {
	if width <= 0 || len(bindings) == 0 {
		return ""
	}
	parts := make([]string, 0, len(bindings)/2)
	for i := 0; i+1 < len(bindings); i += 2 {
		parts = append(parts, systemStyle.Render(bindings[i])+mutedStyle.Render(" "+strings.ToLower(bindings[i+1])))
	}
	line := strings.Join(parts, mutedStyle.Render("   "))
	if ansi.StringWidth(line) <= width {
		return line
	}
	compact := make([]string, 0, len(bindings)/2)
	for i := 0; i+1 < len(bindings); i += 2 {
		compact = append(compact, systemStyle.Render(bindings[i]))
	}
	line = strings.Join(compact, mutedStyle.Render(" · "))
	if ansi.StringWidth(line) <= width {
		return line
	}
	return mutedStyle.Render(truncateWithEllipsis(ansi.Strip(line), maxInt(1, width)))
}

func paneHelpStatusLine(width int, help string, status string) string {
	help = strings.TrimSpace(strings.ReplaceAll(help, "\n", " "))
	status = strings.TrimSpace(strings.ReplaceAll(status, "\n", " "))
	if width <= 0 {
		return ""
	}
	if status == "" {
		return truncateWithEllipsis(help, width)
	}
	if help == "" {
		return paneRightStatus(width, status)
	}
	statusWidth := ansi.StringWidth(status)
	helpWidth := width - statusWidth - 1
	if helpWidth < 4 {
		return paneRightStatus(width, status)
	}
	help = truncateWithEllipsis(help, helpWidth)
	gap := maxInt(1, width-ansi.StringWidth(help)-statusWidth)
	return help + strings.Repeat(" ", gap) + mutedStyle.Render(status)
}

func paneRightStatus(width int, text string) string {
	text = strings.TrimSpace(text)
	if text == "" || width <= 0 {
		return ""
	}
	available := maxInt(1, width-6)
	text = truncateWithEllipsis(text, available)
	padding := maxInt(0, available-ansi.StringWidth(text))
	return strings.Repeat(" ", padding) + mutedStyle.Render(text)
}

func appendPaneGroup(out []string, group ...string) []string {
	// Callers commonly pass overlapping slices such as rows[:1], rows[1:]....
	// Copy the group before appending so inserting the separator cannot clobber it.
	group = append([]string(nil), group...)
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	for len(group) > 0 && strings.TrimSpace(group[0]) == "" {
		group = group[1:]
	}
	for len(group) > 0 && strings.TrimSpace(group[len(group)-1]) == "" {
		group = group[:len(group)-1]
	}
	if len(group) == 0 {
		return out
	}
	if len(out) > 0 {
		out = append(out, "")
	}
	return append(out, group...)
}

func paneSection(title string, rows []string, help string, status string, width int) []string {
	out := []string{brandStyle.Render(title)}
	out = appendPaneGroup(out, rows...)
	if help != "" {
		out = appendPaneGroup(out, help)
	}
	if status != "" {
		out = append(out, paneRightStatus(width, status))
	}
	return out
}

func paneToneColor(tone panecommon.Tone) color.Color {
	switch tone {
	case panecommon.ToneUser:
		return accentUser
	case panecommon.ToneError:
		return accentError
	case panecommon.ToneWarning:
		return warningColor
	default:
		return accentAssistant
	}
}
