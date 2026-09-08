package runtime

import (
	"encoding/json"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/diagnostic"
	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"strings"
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
	modalStyle       = tuistyle.ModalStyle
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

func normalizedPickerWindow(index, offset, count, visible int) (int, int, int) {
	return pane.NormalizedWindow(index, offset, count, visible)
}

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

func NewHistoryState(maxLines int) *HistoryState {
	return tuihistory.NewHistoryState(maxLines)
}

func extractToolTarget(name string, kind tool.Kind, args json.RawMessage) (string, tool.Kind) {
	return toolview.ExtractTarget(name, kind, args)
}

func isAgentLifecycleTool(name string) bool {
	return toolview.IsAgentLifecycleTool(name)
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

type OpenCodeErrorKind = diagnostic.Kind

type ClassifiedError = diagnostic.Error

const (
	ErrorKindModelNotFound    = diagnostic.KindModelNotFound
	ErrorKindContextOverflow  = diagnostic.KindContextOverflow
	ErrorKindAuthentication   = diagnostic.KindAuthentication
	ErrorKindForbidden        = diagnostic.KindForbidden
	ErrorKindRateLimit        = diagnostic.KindRateLimit
	ErrorKindQuotaExceeded    = diagnostic.KindQuotaExceeded
	ErrorKindServerOverloaded = diagnostic.KindServerOverloaded
	ErrorKindStreamTimeout    = diagnostic.KindStreamTimeout
	ErrorKindInvalidPrompt    = diagnostic.KindInvalidPrompt
	ErrorKindMCPFailed        = diagnostic.KindMCPFailed
	ErrorKindConfigInvalid    = diagnostic.KindConfigInvalid
	ErrorKindConfigTypo       = diagnostic.KindConfigTypo
	ErrorKindToolFailed       = diagnostic.KindToolFailed
	ErrorKindToolDispatch     = diagnostic.KindToolDispatch
	ErrorKindPermissionDenied = diagnostic.KindPermissionDenied
	ErrorKindCancelled        = diagnostic.KindCancelled
	ErrorKindGeneric          = diagnostic.KindGeneric
)

func ClassifyOpenCodeError(err error, activeProvider, activeModel string) ClassifiedError {
	return diagnostic.Classify(err, activeProvider, activeModel)
}

func FormatErrorSummary(classified ClassifiedError) string {
	return diagnostic.FormatSummary(classified)
}

type terminalLayoutMode = pane.LayoutMode

const (
	layoutNormal  = pane.LayoutNormal
	layoutCompact = pane.LayoutCompact
	layoutTiny    = pane.LayoutTiny
)

func layoutModeForHeight(height int) terminalLayoutMode {
	return pane.ModeForHeight(height)
}

func pickerVisibleRows(height, maximum int) int {
	return pane.PickerVisibleRows(height, maximum)
}

func compactPickerRows(rows []string) []string {
	return pane.CompactRows(rows)
}

func renderModalRows(m *bubbleModel, border lipgloss.TerminalColor, rows []string) string {
	if m == nil {
		return pane.RenderModal(defaultBubbleWidth, defaultBubbleHeight, border, rows)
	}
	return pane.RenderModal(m.width, m.height, border, rows)
}

func paneToneColor(tone pane.Tone) lipgloss.TerminalColor {
	switch tone {
	case pane.ToneUser:
		return accentUser
	case pane.ToneError:
		return accentError
	case pane.ToneWarning:
		return warningColor
	default:
		return accentAssistant
	}
}
