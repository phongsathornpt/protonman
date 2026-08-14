package tui

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// ANSIScreen renders full-screen frames using standard terminal escape sequences.
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

// Render draws a complete frame and positions the cursor at the input line.
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
	if height < 6 {
		height = 6
	}
	lines := make([]string, 0, height)
	planState := "off"
	if frame.PlanMode {
		planState = "on"
	}
	lines = append(lines, fitLine(" Proton | permission: "+frame.Mode+" | plan: "+planState, width))

	contentHeight := height - 3
	body := make([]string, 0, contentHeight)
	if frame.Modal != nil {
		body = append(body, fitLine("! "+frame.Modal.Title, width))
		for _, bodyLine := range strings.Split(frame.Modal.Body, "\n") {
			body = append(body, fitLine("  "+bodyLine, width))
		}
		body = append(body, fitLine("  "+frame.Modal.Choices, width))
	} else {
		scrollbackStart := len(frame.Scrollback) - contentHeight
		if scrollbackStart < 0 {
			scrollbackStart = 0
		}
		for _, line := range frame.Scrollback[scrollbackStart:] {
			body = append(body, fitLine(line, width))
		}
	}
	if len(body) > contentHeight {
		body = body[len(body)-contentHeight:]
	}
	lines = append(lines, body...)
	for len(lines) < height-2 {
		lines = append(lines, "")
	}

	todoSummary := todoSummary(frame.Todo)
	lines = append(lines, fitLine("─ TODO "+todoSummary, width))
	lines = append(lines, fitLine("> "+frame.Input, width))
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
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
