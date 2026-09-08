package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CrashModel is a fullscreen terminal view shown when Protonman encounters a fatal panic or crash.
// Inspired by OpenCode's error-component.tsx.
type CrashModel struct {
	errMessage   string
	stackTrace   string
	report       string
	copied       bool
	width        int
	height       int
	scrollOffset int
	restart      bool
	quitting     bool
}

// NewCrashModel creates a crash presentation model.
func NewCrashModel(panicVal any, stack []byte) *CrashModel {
	msg := fmt.Sprintf("%v", panicVal)
	if msg == "" {
		msg = "Unexpected runtime panic"
	}
	stackStr := strings.TrimSpace(string(stack))
	if stackStr == "" {
		stackStr = strings.TrimSpace(string(debug.Stack()))
	}
	report := BuildCrashReport(msg, stackStr)

	return &CrashModel{
		errMessage: msg,
		stackTrace: stackStr,
		report:     report,
		width:      80,
		height:     24,
	}
}

// BuildCrashReport formats a GitHub issue bug report with environment context.
func BuildCrashReport(message string, stack string) string {
	var sb strings.Builder
	sb.WriteString("### Protonman Crash Report\n\n")
	sb.WriteString("The Protonman TUI encountered an unexpected error.\n\n")
	sb.WriteString(fmt.Sprintf("**Error:** `%s`\n\n", message))
	sb.WriteString(fmt.Sprintf("**Protonman Version:** `%s`\n", appVersion))
	sb.WriteString(fmt.Sprintf("**Platform:** `%s/%s`\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("**Terminal:** `%s`\n\n", os.Getenv("TERM")))
	sb.WriteString("```\n")
	sb.WriteString(stack)
	sb.WriteString("\n```\n")
	return sb.String()
}

func (m *CrashModel) Init() tea.Cmd {
	return nil
}

func (m *CrashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = maxInt(24, msg.Width)
		m.height = maxInt(10, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "r":
			m.restart = true
			m.quitting = true
			return m, tea.Quit
		case "c":
			_ = copyToClipboard(m.report)
			m.copied = true
			return m, nil
		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			return m, nil
		case "down", "j":
			lines := strings.Split(m.stackTrace, "\n")
			maxScroll := maxInt(0, len(lines)-4)
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
			return m, nil
		case "pgup":
			m.scrollOffset = maxInt(0, m.scrollOffset-5)
			return m, nil
		case "pgdown":
			lines := strings.Split(m.stackTrace, "\n")
			maxScroll := maxInt(0, len(lines)-4)
			m.scrollOffset = minInt(maxScroll, m.scrollOffset+5)
			return m, nil
		}
	}
	return m, nil
}

func (m *CrashModel) View() string {
	contentWidth := minInt(84, maxInt(24, m.width-4))
	innerWidth := contentWidth - 4

	var parts []string

	// 1. Headline
	headline := brandStyle.Render("Protonman crashed")
	subtext := mutedStyle.Render("An unexpected error stopped the session.")
	parts = append(parts, lipgloss.JoinVertical(lipgloss.Center, headline, subtext))

	// 2. Error message panel
	errBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentError).
		Padding(0, 1).
		Width(contentWidth)
	errText := safeWrappedLines(m.errMessage, innerWidth)
	parts = append(parts, errBoxStyle.Render(lipgloss.NewStyle().Foreground(accentError).Bold(true).Render(strings.Join(errText, "\n"))))

	// 3. Actions
	copyLabel := "[c] Copy report"
	if m.copied {
		copyLabel = "[✓ Copied to clipboard]"
	}
	copyStyle := lipgloss.NewStyle().Foreground(accentSuccess).Bold(m.copied)
	if !m.copied {
		copyStyle = lipgloss.NewStyle().Foreground(accentUser)
	}
	actions := lipgloss.JoinHorizontal(
		lipgloss.Center,
		copyStyle.Render(copyLabel),
		"   ",
		commandStyle.Render("[r] Restart"),
		"   ",
		mutedStyle.Render("[q] Quit"),
	)
	parts = append(parts, actions)

	// 4. Stack Trace box (scrollable)
	stackLines := strings.Split(m.stackTrace, "\n")
	availableHeight := maxInt(4, m.height-len(strings.Split(lipgloss.JoinVertical(lipgloss.Left, parts...), "\n"))-4)
	visibleLines := make([]string, 0, availableHeight)

	start := minInt(len(stackLines), m.scrollOffset)
	end := minInt(len(stackLines), start+availableHeight)
	for i := start; i < end; i++ {
		visibleLines = append(visibleLines, stackLines[i])
	}

	stackBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "240", Dark: "8"}).
		Padding(0, 1).
		Width(contentWidth)

	stackHeader := mutedStyle.Render(fmt.Sprintf("Stack trace (lines %d-%d of %d, ↑/↓ scroll):", start+1, end, len(stackLines)))
	stackBody := strings.Join(visibleLines, "\n")
	parts = append(parts, stackBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, stackHeader, mutedStyle.Render(stackBody))))

	// 5. Footer
	footer := mutedStyle.Render(fmt.Sprintf("Protonman %s · %s/%s", appVersion, runtime.GOOS, runtime.GOARCH))
	parts = append(parts, footer)

	mainContent := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, mainContent)
}

func copyToClipboard(text string) error {
	// 1. Terminal standard OSC 52 escape sequence
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	osc52 := fmt.Sprintf("\x1b]52;c;%s\x07", encoded)
	_, _ = os.Stdout.WriteString(osc52)

	// 2. Platform clipboard fallbacks
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.Command("wl-copy")
			cmd.Stdin = strings.NewReader(text)
			_ = cmd.Run()
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd := exec.Command("xclip", "-selection", "clipboard")
			cmd.Stdin = strings.NewReader(text)
			_ = cmd.Run()
		}
	}
	return nil
}
