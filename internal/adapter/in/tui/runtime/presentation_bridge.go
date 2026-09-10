package runtime

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
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
