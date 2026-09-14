package history

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// PatchCell gives edit tools a change-oriented presentation. Paths are best
// effort presentation metadata derived from already-validated tool arguments;
// they never participate in authorization or execution.
type PatchCell struct {
	CallID      string
	Name        string
	Summary     string
	Paths       []string
	Body        string
	Running     bool
	Truncated   bool
	Denied      bool
	FailureCode tool.ErrorCode
	Spinner     string
	Icons       tuistyle.IconSet

	// Retry tracking
	Attempts     int    // Number of attempts (1 for initial, 2+ for retries)
	Retrying     bool   // Whether the patch is currently awaiting or executing a retry
	LastError    string // Diagnostic message from the last failure
	CheckpointID string // Durable pre-edit checkpoint identifier
}

func (PatchCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c PatchCell) RenderWidth(width int) []string {
	width = max(1, width)
	icons := tuistyle.OrUnicodeIcons(c.Icons)
	title := tool.DisplayName(c.Name)
	if strings.TrimSpace(c.Summary) != "" && c.Summary != "1 file" {
		title += " · " + c.Summary
	}
	visiblePaths := c.Paths
	rawTarget := ""
	if len(visiblePaths) == 1 && strings.TrimSpace(visiblePaths[0]) != "" {
		rawTarget = visiblePaths[0]
		visiblePaths = nil
	}

	// 1. Determine glyph, state, styles, and meta text
	var glyph string
	var glyphStyle lipgloss.Style
	var metaText string
	var metaStyle lipgloss.Style

	if c.Running {
		indicator := " …"
		if c.Spinner != "" {
			indicator = " " + c.Spinner
		}
		if c.Retrying {
			glyph = icons.Edit
			glyphStyle = tuistyle.WarningStyle
			retryLabel := "retrying"
			if c.Attempts > 1 {
				retryLabel = fmt.Sprintf("retrying (attempt %d)", c.Attempts)
			}
			metaText = tuistyle.GlyphSep + retryLabel + indicator
			metaStyle = tuistyle.WarningStyle
		} else {
			glyph = icons.Edit
			glyphStyle = tuistyle.FocusStyle
			metaText = indicator
			metaStyle = tuistyle.FocusStyle
		}
	} else if c.Denied {
		glyph = icons.ToolDenied
		glyphStyle = tuistyle.ErrorStyle
		metaText = tuistyle.GlyphSep + "denied"
		metaStyle = tuistyle.ErrorStyle
	} else if c.Retrying && c.FailureCode != "" {
		glyph = icons.ToolError
		glyphStyle = tuistyle.WarningStyle
		retryLabel := "retrying (attempt 1 failed)"
		if c.Attempts > 1 {
			retryLabel = fmt.Sprintf("retrying (attempt %d failed)", c.Attempts)
		}
		metaText = tuistyle.GlyphSep + retryLabel
		metaStyle = tuistyle.WarningStyle
	} else if c.FailureCode != "" {
		glyph = icons.ToolError
		glyphStyle = tuistyle.ErrorStyle
		failLabel := string(c.FailureCode)
		if c.Attempts > 1 {
			failLabel = fmt.Sprintf("%s (failed after %d attempts)", c.FailureCode, c.Attempts)
		}
		metaText = tuistyle.GlyphSep + failLabel
		metaStyle = tuistyle.ErrorStyle
	} else {
		glyph = icons.ToolSuccess
		glyphStyle = tuistyle.SuccessStyle
		if c.Attempts > 1 {
			metaText = " " + fmt.Sprintf("(retried %dx)", c.Attempts-1)
			metaStyle = tuistyle.WarningStyle
		}
	}

	// 2. Format responsive target if singlePath
	targetFormatted := ""
	if rawTarget != "" {
		reserved := ansi.StringWidth(glyph) + ansi.StringWidth(title) + ansi.StringWidth(metaText) + 2
		targetFormatted = " " + toolview.FormatPathWidth(rawTarget, max(6, width-reserved))
	}

	// 3. Assemble header line with calm, semantic styling
	labelStyled := tuistyle.MutedStyle.Render(title)
	headerLine := glyphStyle.Render(glyph) + labelStyled + targetFormatted
	if metaText != "" {
		headerLine += metaStyle.Render(metaText)
	}

	out := make([]string, 0, 2)
	for _, line := range wrapStyledLines(headerLine, width) {
		out = append(out, line)
	}

	// Multi-path rendering (if more than 1 path)
	hiddenPaths := 0
	if len(visiblePaths) > 4 {
		hiddenPaths = len(visiblePaths) - 3
		visiblePaths = visiblePaths[:3]
	}
	for _, path := range visiblePaths {
		styledPath := toolview.FormatPath(path)
		for _, wrapped := range wrapStyledLines(styledPath, max(1, width-2)) {
			out = append(out, "  "+wrapped)
		}
	}
	if hiddenPaths > 0 {
		out = append(out, tuistyle.ToolFoldStyle.Render(fmt.Sprintf("  … (+%d more files · ctrl+t for full list)", hiddenPaths)))
	}

	// 4. Detail lines:
	// A. If failure (active retry OR terminal failure), show compact error excerpt
	if (c.Retrying || c.FailureCode != "") && c.LastError != "" {
		compactErr := strings.TrimSpace(c.LastError)
		if firstLine, _, ok := strings.Cut(compactErr, "\n"); ok {
			compactErr = firstLine
		}
		for _, wrapped := range safeWrappedLines("↳ "+compactErr, max(1, width-2)) {
			out = append(out, tuistyle.ToolExcerptStyle.Render("  "+wrapped))
		}
	} else if !c.Running && c.FailureCode == "" && !c.Denied {
		// B. If success, show compact checkpoint token if present
		cpToken := compactCheckpoint(c.CheckpointID, c.Body)
		if cpToken != "" {
			out = append(out, tuistyle.ToolExcerptStyle.Render("  ↳ checkpoint "+cpToken))
		}
	}

	// C. If body has diff lines (e.g. patch diffs), render them folded
	if !c.Running && c.Body != "" && (c.Denied || c.FailureCode != "" || len(c.Paths) == 0) {
		bodyLines := resultBodyLines(c.Body, nil, c.Truncated, c.Denied, c.FailureCode)
		if len(bodyLines) > 0 {
			folded := toolview.FormatOutputFold(bodyLines, 3)
			for _, line := range folded {
				if styled, isDiff := toolview.StyleDiffLine(line); isDiff {
					for _, wrapped := range safeWrappedLines(styled, max(1, width-2)) {
						out = append(out, "  "+wrapped)
					}
					continue
				}
				for _, wrapped := range safeWrappedLines(line, max(1, width-2)) {
					out = append(out, tuistyle.BodyStyle.Render("  "+wrapped))
				}
			}
		}
	}

	return out
}

func compactCheckpoint(checkpointID, body string) string {
	id := strings.TrimSpace(checkpointID)
	if id == "" && body != "" {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "checkpoint: ") {
				id = strings.TrimSpace(strings.TrimPrefix(line, "checkpoint: "))
				break
			}
		}
	}
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, "checkpoint-") {
		parts := strings.Split(id, "-")
		if len(parts) >= 3 {
			hash := parts[2]
			if len(hash) > 8 {
				hash = hash[:8]
			}
			return "chk-" + hash
		}
	}
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
func (c PatchCell) RawLines() []string {
	title := tool.DisplayName(c.Name)
	if strings.TrimSpace(c.Summary) != "" && c.Summary != "1 file" {
		title += " · " + c.Summary
	}
	out := []string{tuistyle.GlyphEdit + sanitizeBubbleText(title)}
	for _, path := range c.Paths {
		out = append(out, sanitizeBubbleText(path))
	}
	if c.Attempts > 1 {
		out = append(out, fmt.Sprintf("attempts: %d", c.Attempts))
	}
	out = append(out, resultBodyLines(c.Body, nil, c.Truncated, c.Denied, c.FailureCode)...)
	return out
}
func (c PatchCell) LineCount() int           { return len(c.RawLines()) }
func (c PatchCell) historyToolID() string    { return c.CallID }
func (c PatchCell) historyToolName() string  { return c.Name }
func (c PatchCell) historyToolRunning() bool { return c.Running }
