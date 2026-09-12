package runtime

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/diagnostic"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
)

func brandLockup(width int) string {
	return tuistyle.BrandLockup(width)
}

func brandLockupWidth(width int) int {
	return tuistyle.BrandLockupWidth(width)
}

const (
	glyphPrompt      = tuistyle.GlyphPrompt
	glyphMark        = tuistyle.GlyphMark
	glyphTool        = tuistyle.GlyphTool
	glyphSep         = tuistyle.GlyphSep
	glyphToolSuccess = tuistyle.GlyphToolSuccess
	glyphToolError   = tuistyle.GlyphToolError
	glyphToolDenied  = tuistyle.GlyphToolDenied
	glyphWeb         = tuistyle.GlyphWeb
	glyphRead        = tuistyle.GlyphRead
	glyphDir         = tuistyle.GlyphDir
	glyphSearch      = tuistyle.GlyphSearch
	glyphExec        = tuistyle.GlyphExec
	glyphEdit        = tuistyle.GlyphEdit
	glyphSkill       = tuistyle.GlyphSkill
	glyphAgent       = tuistyle.GlyphAgent
	glyphGeneric     = tuistyle.GlyphGeneric
	glyphTodoPending = tuistyle.GlyphTodoPending
	glyphTodoActive  = tuistyle.GlyphTodoActive
	glyphBrand       = tuistyle.GlyphBrand
)

var (
	appVersion       = buildinfo.Version()
	accentAssistant  = tuistyle.AccentAssistant
	accentUser       = tuistyle.AccentUser
	accentTool       = tuistyle.AccentTool
	accentSystem     = tuistyle.AccentSystem
	accentPlan       = tuistyle.AccentPlan
	accentError      = tuistyle.AccentError
	accentSuccess    = tuistyle.AccentSuccess
	commandColor     = tuistyle.CommandColor
	warningColor     = tuistyle.WarningColor
	promptBorder     = tuistyle.PromptBorder
	brandStyle       = tuistyle.BrandStyle
	brandMarkStyle   = tuistyle.BrandMarkStyle
	userStyle        = tuistyle.UserStyle
	assistantStyle   = tuistyle.AssistantStyle
	toolStyle        = tuistyle.ToolStyle
	systemStyle      = tuistyle.SystemStyle
	mutedStyle       = tuistyle.MutedStyle
	statusStyle      = tuistyle.StatusStyle
	warningStyle     = tuistyle.WarningStyle
	successStyle     = tuistyle.SuccessStyle
	errorStyle       = tuistyle.ErrorStyle
	planStyle        = tuistyle.PlanStyle
	commandStyle     = tuistyle.CommandStyle
	toolTargetStyle  = tuistyle.ToolTargetStyle
	toolDirStyle     = tuistyle.ToolDirStyle
	toolSummaryStyle = tuistyle.ToolSummaryStyle
	fileBadgeStyle   = tuistyle.FileBadgeStyle
	toolExcerptStyle = tuistyle.ToolExcerptStyle
	toolFoldStyle    = tuistyle.ToolFoldStyle
	heroLabelStyle   = tuistyle.HeroLabelStyle
	heroKeyStyle     = tuistyle.HeroKeyStyle
	diffAddStyle     = tuistyle.DiffAddStyle
	diffDeleteStyle  = tuistyle.DiffDeleteStyle
	diffHunkStyle    = tuistyle.DiffHunkStyle
	bodyStyle        = tuistyle.BodyStyle
)

var (
	markdownCodeStyle = tuistyle.MarkdownCodeStyle
	markdownBoldStyle = tuistyle.MarkdownBoldStyle
)

type simpleANSIStyle struct{ inner textview.SimpleANSIStyle }

func (s *simpleANSIStyle) writeTo(out *strings.Builder, style lipgloss.Style, text string) {
	s.inner.WriteTo(out, style, text)
}

func renderMarkdownLines(markdown string, width int) []string {
	return textview.RenderMarkdownLines(markdown, width)
}

func renderMarkdownBodyWrapped(text string, width int) []string {
	return textview.RenderMarkdownBodyWrapped(text, width)
}

func renderMarkdownWrapped(text string, width int, style lipgloss.Style) []string {
	return textview.RenderMarkdownWrapped(text, width, style)
}

type markdownRenderState = textview.MarkdownState

func renderMarkdownLine(raw string, width int, state *markdownRenderState) []string {
	return textview.RenderMarkdownLine(raw, width, state)
}

func trimTrailingBlankLines(lines []string) []string {
	return textview.TrimTrailingBlankLines(lines)
}

func wrapWords(text string, width int) string {
	return textview.WrapWords(text, width)
}

func wrapLines(text string, width int) []string {
	return textview.WrapLines(text, width)
}

func sanitizeBubbleText(text string) string {
	return textview.Sanitize(text)
}

func safeWrappedLines(text string, width int) []string {
	return textview.SafeWrappedLines(text, width)
}

func overlayCenter(background, overlay string, width, height int) string {
	if width <= 0 || height <= 0 {
		return overlay
	}
	bgLines := strings.Split(background, "\n")
	lines := make([]string, height)
	for i := range lines {
		line := ""
		if i < len(bgLines) {
			line = bgLines[i]
		}
		lines[i] = padVisual(line, width)
	}
	fgLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")
	fgHeight := len(fgLines)
	if fgHeight > height {
		fgLines = fgLines[:height]
		fgHeight = height
	}
	fgWidth := 0
	for _, line := range fgLines {
		if w := ansi.StringWidth(line); w > fgWidth {
			fgWidth = w
		}
	}
	if fgWidth > width {
		fgWidth = width
	}
	top := (height - fgHeight) / 2
	left := (width - fgWidth) / 2
	if top < 0 {
		top = 0
	}
	if left < 0 {
		left = 0
	}
	for i := 0; i < fgHeight; i++ {
		fg := ansi.Truncate(fgLines[i], fgWidth, "")
		if w := ansi.StringWidth(fg); w < fgWidth {
			fg += strings.Repeat(" ", fgWidth-w)
		}
		lines[top+i] = spliceVisual(lines[top+i], fg, left, width)
	}
	return strings.Join(lines, "\n")
}

func padVisual(line string, width int) string {
	visual := ansi.StringWidth(line)
	if visual == width {
		return line
	}
	if visual > width {
		return ansi.Truncate(line, width, "")
	}
	return line + strings.Repeat(" ", width-visual)
}

func spliceVisual(dst, src string, left, width int) string {
	if left < 0 {
		left = 0
	}
	srcWidth := ansi.StringWidth(src)
	if left+srcWidth > width {
		src = ansi.Truncate(src, maxInt(0, width-left), "")
		srcWidth = ansi.StringWidth(src)
	}
	prefix := ansi.Cut(dst, 0, left)
	suffix := ""
	if end := left + srcWidth; end < width {
		suffix = ansi.Cut(dst, end, width)
	}
	return padVisual(prefix+src+suffix, width)
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

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
	return paneutil.Window(count, selected, maximum, mode)
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
