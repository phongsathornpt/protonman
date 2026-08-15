package tui

import "github.com/charmbracelet/lipgloss"

const (
	glyphPrompt = "❯ "
	glyphMark   = "◆ "
	glyphTool   = "$ "
	glyphSep    = " · "
	appVersion  = "dev"
)

var (
	accentAssistant = lipgloss.AdaptiveColor{Light: "#7D4BC6", Dark: "#BB9AF7"}
	accentUser      = lipgloss.AdaptiveColor{Light: "#444444", Dark: "#C8C8C8"}
	accentTool      = lipgloss.AdaptiveColor{Light: "#626262", Dark: "#787878"}
	accentSystem    = lipgloss.AdaptiveColor{Light: "#2F64D2", Dark: "#7AA2F7"}
	accentPlan      = lipgloss.AdaptiveColor{Light: "#A27612", Dark: "#FFDB8D"}
	accentError     = lipgloss.AdaptiveColor{Light: "#CD3048", Dark: "#F7768E"}
	accentSuccess   = lipgloss.AdaptiveColor{Light: "#378E23", Dark: "#9ECE6A"}
	commandColor    = lipgloss.AdaptiveColor{Light: "#A27612", Dark: "#E0AF68"}
	warningColor    = lipgloss.AdaptiveColor{Light: "#A27612", Dark: "#E0AF68"}
	textPrimary     = lipgloss.AdaptiveColor{Light: "#262626", Dark: "#E1E1E1"}
	mutedColor      = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#6C6C6C"}
	promptBorder    = lipgloss.AdaptiveColor{Light: "#B2B2B2", Dark: "#323237"}
)

var (
	brandStyle     = lipgloss.NewStyle().Bold(true).Foreground(accentAssistant)
	userStyle      = lipgloss.NewStyle().Foreground(accentUser)
	assistantStyle = lipgloss.NewStyle().Foreground(accentAssistant)
	toolStyle      = lipgloss.NewStyle().Foreground(accentTool)
	systemStyle    = lipgloss.NewStyle().Foreground(accentSystem)
	mutedStyle     = lipgloss.NewStyle().Foreground(mutedColor)
	statusStyle    = lipgloss.NewStyle().Foreground(accentAssistant)
	warningStyle   = lipgloss.NewStyle().Foreground(warningColor)
	successStyle   = lipgloss.NewStyle().Foreground(accentSuccess)
	errorStyle     = lipgloss.NewStyle().Foreground(accentError)
	planStyle      = lipgloss.NewStyle().Foreground(accentPlan)
	commandStyle   = lipgloss.NewStyle().Foreground(commandColor)
	bodyStyle      = lipgloss.NewStyle().Foreground(textPrimary)
	modalStyle     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(warningColor).
			Padding(1, 2)
)
