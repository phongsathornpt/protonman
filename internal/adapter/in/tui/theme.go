package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
)

const (
	glyphPrompt      = "› "
	glyphMark        = "› "
	glyphTool        = "$ "
	glyphSep         = " · "
	glyphToolSuccess = "✓ "
	glyphToolError   = "× "
	glyphToolDenied  = "! "
	glyphWeb         = "↗ "
	glyphRead        = "≡ "
	glyphDir         = "▸ "
	glyphSearch      = "? "
	glyphExec        = "$ "
	glyphEdit        = "+ "
	glyphSkill       = "* "
	glyphAgent       = "→ "
	glyphGeneric     = "· "
	glyphTodoPending = "○ "
	glyphTodoActive  = "● "
	// glyphBrand is the compact Protonman terminal mark.
	glyphBrand = "◆"
)

// Prefer terminal-native ANSI colors so Protonman remains readable across light,
// dark and customized terminal themes. Primary body text intentionally uses
// the terminal's default foreground.
var (
	appVersion      = buildinfo.Version()
	accentAssistant = lipgloss.Color("5")                             // magenta: Protonman identity
	accentUser      = lipgloss.Color("6")                             // cyan: input/selection
	accentTool      = lipgloss.AdaptiveColor{Light: "240", Dark: "8"} // dim tool chrome
	accentSystem    = lipgloss.Color("6")                             // cyan: status/info
	accentPlan      = lipgloss.Color("6")
	accentError     = lipgloss.Color("1") // red
	accentSuccess   = lipgloss.Color("2") // green
	commandColor    = lipgloss.Color("6")
	warningColor    = lipgloss.Color("3") // yellow: warnings, attention, denied
	promptBorder    = lipgloss.AdaptiveColor{Light: "242", Dark: "8"}
)

var (
	brandStyle       = lipgloss.NewStyle().Bold(true).Foreground(accentAssistant)
	brandMarkStyle   = lipgloss.NewStyle().Foreground(accentAssistant)
	userStyle        = lipgloss.NewStyle().Foreground(accentUser)
	assistantStyle   = lipgloss.NewStyle()
	toolStyle        = lipgloss.NewStyle().Foreground(accentTool)
	systemStyle      = lipgloss.NewStyle().Foreground(accentSystem)
	mutedStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "238", Dark: "7"})
	statusStyle      = lipgloss.NewStyle().Foreground(accentSystem)
	warningStyle     = lipgloss.NewStyle().Foreground(warningColor)
	successStyle     = lipgloss.NewStyle().Foreground(accentSuccess)
	errorStyle       = lipgloss.NewStyle().Foreground(accentError)
	planStyle        = lipgloss.NewStyle().Foreground(accentPlan)
	commandStyle     = lipgloss.NewStyle().Foreground(commandColor)
	toolTargetStyle  = lipgloss.NewStyle().Bold(true).Foreground(accentUser)
	toolDirStyle     = lipgloss.NewStyle().Foreground(accentTool)
	toolSummaryStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "8"})
	fileBadgeStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "8"})
	toolExcerptStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "8"}).Italic(true)
	toolFoldStyle    = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "8"})
	heroLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "8"}).Bold(true)
	heroKeyStyle     = lipgloss.NewStyle().Foreground(accentUser).Bold(true)
	diffAddStyle     = lipgloss.NewStyle().Foreground(accentSuccess)
	diffDeleteStyle  = lipgloss.NewStyle().Foreground(accentError)
	diffHunkStyle    = lipgloss.NewStyle().Foreground(accentUser)
	bodyStyle        = lipgloss.NewStyle()
	modalStyle       = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(warningColor).
				Padding(1, 2)
)
