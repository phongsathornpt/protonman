package tui

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiCyan   = "\x1b[36m"
	ansiYellow = "\x1b[33m"
	ansiGreen  = "\x1b[32m"
	ansiRed    = "\x1b[31m"
	ansiBlue   = "\x1b[34m"

	fullscreenFooterRows = 4
	maxTodoRows          = 4
)

// ANSIScreen renders the Grok-inspired live-region layout using terminal
// escape sequences: bottom-anchored output, TODOs, status, prompt, and hints.
type ANSIScreen struct {
	writer io.Writer
	width  int
	height int
}

// NewANSIScreen creates an alternate-screen renderer with a fixed fallback size.
func NewANSIScreen(writer io.Writer, width int, height int) (*ANSIScreen, error) {
	if writer == nil {
		return nil, errors.New("ANSI screen writer is required")
	}
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return &ANSIScreen{
		writer: writer,
		width:  width,
		height: height,
	}, nil
}

// Enter switches to the alternate screen and clears the viewport.
func (s *ANSIScreen) Enter() error {
	_, err := io.WriteString(s.writer, "\x1b[?1049h\x1b[2J\x1b[H\x1b[?25h")
	if err != nil {
		return fmt.Errorf("enter ANSI screen: %w", err)
	}
	return nil
}

// Exit restores the normal screen and terminal cursor.
func (s *ANSIScreen) Exit() error {
	_, err := io.WriteString(s.writer, "\x1b[0m\x1b[?25h\x1b[?1049l")
	if err != nil {
		return fmt.Errorf("exit ANSI screen: %w", err)
	}
	return nil
}

// Size returns the configured viewport dimensions.
func (s *ANSIScreen) Size() (int, int) {
	return s.width, s.height
}

// Render draws a complete frame and positions the cursor at the prompt.
func (s *ANSIScreen) Render(frame Frame) error {
	lines := renderFrame(frame, s.width, s.height)
	var output strings.Builder
	output.WriteString("\x1b[H\x1b[2J")
	for index, line := range lines {
		if index > 0 {
			output.WriteByte('\n')
		}
		output.WriteString(line)
	}
	cursorColumn := 3 + frame.Cursor
	if cursorColumn < 1 {
		cursorColumn = 1
	}
	if cursorColumn > s.width {
		cursorColumn = s.width
	}
	output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", s.height, cursorColumn))
	if _, err := io.WriteString(s.writer, output.String()); err != nil {
		return fmt.Errorf("render ANSI screen: %w", err)
	}
	return nil
}

func renderFrame(frame Frame, width int, height int) []string {
	if width <= 0 {
		width = 80
	}
	if height < fullscreenFooterRows+2 {
		height = fullscreenFooterRows + 2
	}

	bodyHeight := height - fullscreenFooterRows
	body := make([]string, 0, bodyHeight)
	if frame.Modal != nil {
		body = append(body, renderModalLines(*frame.Modal, width)...)
	} else {
		todoLines := renderTodoLines(frame.Todo, width)
		scrollbackHeight := bodyHeight - len(todoLines)
		if scrollbackHeight < 0 {
			scrollbackHeight = 0
		}
		body = append(body, renderScrollback(frame.Scrollback, width, scrollbackHeight)...)
		body = append(body, todoLines...)
	}
	if len(body) > bodyHeight {
		body = body[len(body)-bodyHeight:]
	}
	for len(body) < bodyHeight {
		body = append([]string{""}, body...)
	}

	lines := append([]string{}, body...)
	lines = append(lines, renderStatusLine(frame, width))
	lines = append(lines, renderPromptLine(frame.Input, width))
	lines = append(lines, renderInfoLine(frame, width))
	lines = append(lines, styledLine(
		"↑↓ history · ←→ move · Ctrl-C clear · :help commands",
		width,
		ansiDim,
	))
	return lines
}

func renderScrollback(scrollback []string, width int, height int) []string {
	if height <= 0 || len(scrollback) == 0 {
		return []string{}
	}
	start := len(scrollback) - height
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(scrollback)-start)
	for _, line := range scrollback[start:] {
		clean := sanitizeTerminalText(line)
		style := ansiDim
		switch {
		case strings.HasPrefix(clean, "> "):
			style = ansiCyan
		case strings.HasPrefix(clean, "tool "), strings.HasPrefix(clean, "tool:"):
			style = ansiYellow
		case strings.HasPrefix(clean, "error:"), strings.HasPrefix(clean, "turn failed:"):
			style = ansiRed
		case strings.HasPrefix(clean, "assistant:"):
			style = ansiBlue
		}
		lines = append(lines, styledLine(clean, width, style))
	}
	return lines
}

func renderTodoLines(items []TodoItem, width int) []string {
	if len(items) == 0 || maxTodoRows == 0 {
		return []string{}
	}
	visible := len(items)
	if visible > maxTodoRows {
		visible = maxTodoRows
	}
	lines := make([]string, 0, visible+1)
	lines = append(lines, styledLine("TODO "+todoSummary(items), width, ansiBold+ansiCyan))
	for _, item := range items[:visible] {
		mark := "□"
		style := ansiDim
		if item.Done {
			mark = "✓"
			style = ansiGreen
		}
		text := "  " + mark + " " + sanitizeTerminalText(item.Text)
		lines = append(lines, styledLine(text, width, style))
	}
	if len(items) > visible {
		lines = append(lines, styledLine(
			fmt.Sprintf("  … %d more", len(items)-visible),
			width,
			ansiDim,
		))
	}
	return lines
}

func renderModalLines(modal Modal, width int) []string {
	lines := []string{styledLine("╭─ "+sanitizeTerminalText(modal.Title), width, ansiBold+ansiYellow)}
	for _, line := range strings.Split(modal.Body, "\n") {
		lines = append(lines, styledLine("│ "+sanitizeTerminalText(line), width, ansiYellow))
	}
	lines = append(lines, styledLine("│ "+sanitizeTerminalText(modal.Choices), width, ansiBold+ansiYellow))
	lines = append(lines, styledLine("╰─", width, ansiYellow))
	return lines
}

func renderStatusLine(frame Frame, width int) string {
	activity := strings.TrimSpace(frame.Activity)
	if activity == "" {
		activity = "idle"
	}
	planState := "plan off"
	if frame.PlanMode {
		planState = "plan on"
	}
	text := "· " + activity + " · permission: " + frame.Mode + " · " + planState
	return styledLine(text, width, ansiCyan)
}

func renderPromptLine(input string, width int) string {
	return styledLine("❯ "+sanitizeTerminalText(input), width, ansiBold+ansiCyan)
}

func renderInfoLine(frame Frame, width int) string {
	planState := "plan off"
	if frame.PlanMode {
		planState = "plan"
	}
	return styledLine("proton · "+planState+" · :help", width, ansiDim)
}

func styledLine(text string, width int, style string) string {
	return style + fitLine(text, width) + ansiReset
}

func sanitizeTerminalText(text string) string {
	var builder strings.Builder
	state := byte(0)
	for _, value := range text {
		if state == 0 {
			switch {
			case value == 0x1b:
				state = 1
			case value < 0x20 || value == 0x7f:
				builder.WriteByte(' ')
			default:
				builder.WriteRune(value)
			}
			continue
		}
		switch state {
		case 1:
			switch value {
			case '[':
				state = 2
			case ']':
				state = 3
			default:
				state = 0
			}
		case 2:
			if value >= '@' && value <= '~' {
				state = 0
			}
		case 3:
			switch value {
			case 0x07:
				state = 0
			case 0x1b:
				state = 4
			}
		case 4:
			if value == '\\' {
				state = 0
			} else {
				state = 3
			}
		}
	}
	return builder.String()
}

func todoSummary(items []TodoItem) string {
	if len(items) == 0 {
		return "empty"
	}
	done := 0
	for _, item := range items {
		if item.Done {
			done++
		}
	}
	return fmt.Sprintf("%d/%d complete", done, len(items))
}

func fitLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) > width {
		return string(runes[:width])
	}
	return line + strings.Repeat(" ", width-len(runes))
}
