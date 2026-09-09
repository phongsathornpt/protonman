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

// Prefer terminal-native ANSI colors so Protonman remains readable across light,
// dark and customized terminal themes. Primary body text intentionally uses
// the terminal's default foreground.
var (
	AccentAssistant = lipgloss.Color("5") // magenta: Protonman identity
	AccentUser      = lipgloss.Color("6") // cyan: input/selection
	AccentTool      = lipgloss.Color("8") // dim tool chrome
	AccentSystem    = lipgloss.Color("6") // cyan: status/info
	AccentPlan      = lipgloss.Color("6")
	AccentError     = lipgloss.Color("1") // red
	AccentSuccess   = lipgloss.Color("2") // green
	CommandColor    = lipgloss.Color("6")
	WarningColor    = lipgloss.Color("3") // yellow: warnings, attention, denied
	PromptBorder    = lipgloss.Color("8")
)

var (
	BrandStyle           = lipgloss.NewStyle().Bold(true).Foreground(AccentAssistant)
	BrandMarkStyle       = lipgloss.NewStyle().Foreground(AccentAssistant)
	UserStyle            = lipgloss.NewStyle().Foreground(AccentUser)
	AssistantStyle       = lipgloss.NewStyle()
	ToolStyle            = lipgloss.NewStyle().Foreground(AccentTool)
	SystemStyle          = lipgloss.NewStyle().Foreground(AccentSystem)
	MutedStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	StatusStyle          = lipgloss.NewStyle().Foreground(AccentSystem)
	WarningStyle         = lipgloss.NewStyle().Foreground(WarningColor)
	SuccessStyle         = lipgloss.NewStyle().Foreground(AccentSuccess)
	ErrorStyle           = lipgloss.NewStyle().Foreground(AccentError)
	PlanStyle            = lipgloss.NewStyle().Foreground(AccentPlan)
	CommandStyle         = lipgloss.NewStyle().Foreground(CommandColor)
	ToolTargetStyle      = lipgloss.NewStyle().Bold(true).Foreground(AccentUser)
	ToolDirStyle         = lipgloss.NewStyle().Foreground(AccentTool)
	ToolSummaryStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	FileBadgeStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	ToolExcerptStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	ToolFoldStyle        = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("8"))
	HeroLabelStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)
	HeroKeyStyle         = lipgloss.NewStyle().Foreground(AccentUser).Bold(true)
	DiffAddStyle         = lipgloss.NewStyle().Foreground(AccentSuccess)
	DiffDeleteStyle      = lipgloss.NewStyle().Foreground(AccentError)
	DiffHunkStyle        = lipgloss.NewStyle().Foreground(AccentUser)
	BodyStyle            = lipgloss.NewStyle()
	MarkdownHeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(AccentAssistant)
	MarkdownCodeStyle    = lipgloss.NewStyle().Foreground(AccentSystem)
	MarkdownQuoteStyle   = lipgloss.NewStyle().Foreground(AccentTool)
	MarkdownBulletStyle  = lipgloss.NewStyle().Foreground(AccentAssistant)
	MarkdownBoldStyle    = lipgloss.NewStyle().Bold(true)
	ModalStyle           = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(WarningColor).
				Padding(1, 2)
)
