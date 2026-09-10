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

// Protonman CLI follows the dark product palette used by protonman-rs.
// Keep these as semantic tokens so presentation code does not grow its own
// collection of nearly-identical oranges and grays.
var (
	ColorTextPrimary   = lipgloss.Color("#f5f5f6")
	ColorTextSecondary = lipgloss.Color("#a8adb8")
	ColorTextTertiary  = lipgloss.Color("#8f96a3")
	ColorPrimary       = lipgloss.Color("#f0983c")
	ColorPrimaryHover  = lipgloss.Color("#ffab5c")
	ColorPrimaryDark   = lipgloss.Color("#ea580c")
	ColorSuccess       = lipgloss.Color("#22c55e")
	ColorWarning       = lipgloss.Color("#eab308")
	ColorDanger        = lipgloss.Color("#f87171")
	ColorBorder        = lipgloss.Color("#2a2a2f")
	ColorBorderSubtle  = lipgloss.Color("#1d1d21")

	AccentAssistant = ColorPrimary
	AccentUser      = ColorPrimaryHover
	AccentTool      = ColorTextTertiary
	AccentSystem    = ColorTextSecondary
	AccentPlan      = ColorPrimary
	AccentError     = ColorDanger
	AccentSuccess   = ColorSuccess
	CommandColor    = ColorPrimaryHover
	WarningColor    = ColorWarning
	PromptBorder    = ColorBorder
)

var (
	BrandStyle           = lipgloss.NewStyle().Bold(true).Foreground(AccentAssistant)
	BrandMarkStyle       = lipgloss.NewStyle().Foreground(AccentAssistant)
	UserStyle            = lipgloss.NewStyle().Foreground(AccentUser)
	AssistantStyle       = lipgloss.NewStyle().Foreground(ColorTextPrimary)
	ToolStyle            = lipgloss.NewStyle().Foreground(AccentTool)
	SystemStyle          = lipgloss.NewStyle().Foreground(AccentSystem)
	MutedStyle           = lipgloss.NewStyle().Foreground(ColorTextTertiary)
	StatusStyle          = lipgloss.NewStyle().Foreground(AccentSystem)
	WarningStyle         = lipgloss.NewStyle().Foreground(WarningColor)
	SuccessStyle         = lipgloss.NewStyle().Foreground(AccentSuccess)
	ErrorStyle           = lipgloss.NewStyle().Foreground(AccentError)
	PlanStyle            = lipgloss.NewStyle().Foreground(AccentPlan)
	CommandStyle         = lipgloss.NewStyle().Foreground(CommandColor)
	ToolTargetStyle      = lipgloss.NewStyle().Bold(true).Foreground(ColorTextSecondary)
	ToolDirStyle         = lipgloss.NewStyle().Foreground(AccentTool)
	ToolSummaryStyle     = lipgloss.NewStyle().Foreground(ColorTextTertiary)
	FileBadgeStyle       = lipgloss.NewStyle().Foreground(ColorTextTertiary)
	ToolExcerptStyle     = lipgloss.NewStyle().Foreground(ColorTextTertiary).Italic(true)
	ToolFoldStyle        = lipgloss.NewStyle().Italic(true).Foreground(ColorTextTertiary)
	HeroLabelStyle       = lipgloss.NewStyle().Foreground(ColorTextTertiary).Bold(true)
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
