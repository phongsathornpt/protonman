package style

import (
	"charm.land/lipgloss/v2"
)

const (
	GlyphPrompt      = "› "
	GlyphMark        = "› "
	GlyphTool        = "$ "
	GlyphSep         = " · "
	GlyphToolSuccess = "✓ "
	GlyphToolError   = "× "
	GlyphToolDenied  = "! "
	GlyphWeb         = "↗ "
	GlyphRead        = "≡ "
	GlyphDir         = "▸ "
	GlyphSearch      = "? "
	GlyphExec        = "$ "
	GlyphEdit        = "+ "
	GlyphSkill       = "* "
	GlyphAgent       = "→ "
	GlyphGeneric     = "· "
	GlyphTodoPending = "○ "
	GlyphTodoActive  = "● "
	// GlyphBrand is the compact Protonman terminal mark.
	GlyphBrand = "◆"
)

// MeasureProse caps prose line length so long model output stays readable on
// ultra-wide terminals. Fenced code, tables, and tool output keep the full
// available width because their alignment carries meaning.
const MeasureProse = 100

// Styles consume semantic color tokens from semantic.go. Keep component
// styling here and palette decisions in palette.go so changing the product
// palette does not require touching renderers.
var (
	BrandStyle           = lipgloss.NewStyle().Bold(true).Foreground(AccentAssistant)
	BrandMarkStyle       = lipgloss.NewStyle().Foreground(AccentAssistant)
	UserStyle            = lipgloss.NewStyle().Foreground(AccentUser)
	AssistantStyle       = lipgloss.NewStyle().Foreground(ColorTextPrimary)
	ToolStyle            = lipgloss.NewStyle().Foreground(AccentTool)
	SystemStyle          = lipgloss.NewStyle().Foreground(AccentSystem)
	MutedStyle           = lipgloss.NewStyle().Foreground(ColorTextMuted)
	StatusStyle          = lipgloss.NewStyle().Foreground(AccentSystem)
	InfoStyle            = lipgloss.NewStyle().Foreground(AccentInfo)
	WarningStyle         = lipgloss.NewStyle().Foreground(WarningColor)
	SuccessStyle         = lipgloss.NewStyle().Foreground(AccentSuccess)
	ErrorStyle           = lipgloss.NewStyle().Foreground(AccentError)
	PlanStyle            = lipgloss.NewStyle().Foreground(AccentPlan)
	CommandStyle         = lipgloss.NewStyle().Foreground(CommandColor)
	FocusStyle           = lipgloss.NewStyle().Foreground(ColorFocus)
	PromptIdleStyle      = lipgloss.NewStyle().Foreground(ColorTextPrimary).BorderForeground(ColorBorder)
	PromptFocusedStyle   = lipgloss.NewStyle().Foreground(ColorTextPrimary).BorderForeground(ColorBorderFocus)
	PromptWarningStyle   = lipgloss.NewStyle().Foreground(ColorTextPrimary).BorderForeground(ColorWarning)
	PromptErrorStyle     = lipgloss.NewStyle().Foreground(ColorTextPrimary).BorderForeground(ColorDanger)
	ToolTargetStyle      = lipgloss.NewStyle().Bold(true).Foreground(ColorTextSecondary)
	ToolDirStyle         = lipgloss.NewStyle().Foreground(AccentTool)
	ToolSummaryStyle     = lipgloss.NewStyle().Foreground(ColorTextMuted)
	FileBadgeStyle       = lipgloss.NewStyle().Foreground(ColorTextMuted)
	ToolExcerptStyle     = lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true)
	ToolFoldStyle        = lipgloss.NewStyle().Italic(true).Foreground(ColorTextMuted)
	HeroLabelStyle       = lipgloss.NewStyle().Foreground(ColorTextMuted).Bold(true)
	HeroKeyStyle         = lipgloss.NewStyle().Foreground(AccentUser).Bold(true)
	DiffAddStyle         = lipgloss.NewStyle().Foreground(AccentSuccess)
	DiffDeleteStyle      = lipgloss.NewStyle().Foreground(AccentError)
	DiffHunkStyle        = lipgloss.NewStyle().Foreground(ColorTextSecondary)
	BodyStyle            = lipgloss.NewStyle().Foreground(ColorTextPrimary)
	MarkdownHeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorTextPrimary)
	MarkdownCodeStyle    = lipgloss.NewStyle().Foreground(CommandColor)
	MarkdownQuoteStyle   = lipgloss.NewStyle().Foreground(ColorTextSecondary)
	MarkdownBulletStyle  = lipgloss.NewStyle().Foreground(ColorTextSecondary)
	MarkdownBoldStyle    = lipgloss.NewStyle().Bold(true)
	ModalStyle           = lipgloss.NewStyle().Padding(0, 1)
)

// Heading scale. Type size is fixed by the terminal, so hierarchy comes from
// weight plus the text-color ladder: MarkdownHeadingStyle is the level-one
// heading, and levels two and three step down that ladder.
var (
	MarkdownH2Style     = lipgloss.NewStyle().Bold(true).Foreground(ColorTextSecondary)
	MarkdownH3Style     = lipgloss.NewStyle().Bold(true).Foreground(ColorTextMuted)
	MarkdownItalicStyle = lipgloss.NewStyle().Italic(true).Foreground(ColorTextSecondary)
)
