package toolview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// HeaderState is the presentation lifecycle of one tool call.
type HeaderState uint8

const (
	HeaderRunning HeaderState = iota
	HeaderSuccess
	HeaderDenied
	HeaderFailure
)

// HeaderInput contains semantic tool facts. Label can override the normal tool
// display name for intentionally specialized surfaces such as skill activation.
type HeaderInput struct {
	Name        string
	Kind        tool.Kind
	Label       string
	Target      string
	Summary     string
	Running     bool
	Denied      bool
	FailureCode tool.ErrorCode
	Spinner     string
}

// Header is the normalized primary-line model for a tool call.
type Header struct {
	State  HeaderState
	Glyph  string
	Label  string
	Target string
	Meta   string
}

// ProjectHeader applies one precedence and wording policy for tool call headers.
func ProjectHeader(input HeaderInput) Header {
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = tool.DisplayName(input.Name)
	}
	header := Header{
		State:  HeaderSuccess,
		Glyph:  tuistyle.GlyphToolSuccess,
		Label:  label,
		Target: strings.TrimSpace(input.Target),
		Meta:   strings.TrimSpace(input.Summary),
	}
	switch {
	case input.Running:
		header.State = HeaderRunning
		header.Glyph = KindGlyph(input.Kind, input.Name)
		header.Meta = strings.TrimSpace(input.Spinner)
		if header.Meta == "" {
			header.Meta = "…"
		}
	case input.Denied:
		header.State = HeaderDenied
		header.Glyph = tuistyle.GlyphToolDenied
		header.Meta = "denied"
	case input.FailureCode != "":
		header.State = HeaderFailure
		header.Glyph = tuistyle.GlyphToolError
		header.Meta = FailureLabel(input.Name, input.Kind, input.FailureCode)
	}
	return header
}

// FailureLabel converts stable internal failure codes into concise human-facing
// copy. Structured codes remain unchanged in tool results and diagnostics.
func FailureLabel(name string, kind tool.Kind, code tool.ErrorCode) string {
	switch code {
	case tool.ErrorCodeInvalidArguments:
		return "invalid arguments"
	case tool.ErrorCodeCommandFailed:
		return "command failed"
	case tool.ErrorCodeInvalidOutput:
		return "invalid output"
	case tool.ErrorCodeOutputTooLarge:
		return "output too large"
	case tool.ErrorCodeCanceled:
		return "canceled"
	case tool.ErrorCodeDeadlineExceeded:
		return "timed out"
	case tool.ErrorCodePermissionDenied:
		return "permission denied"
	case tool.ErrorCodeUnknownTool:
		return "unknown tool"
	case tool.ErrorCodeNotFound:
		switch strings.TrimSpace(name) {
		case tool.NameRead:
			return "file not found"
		case tool.NameLS:
			return "path not found"
		}
		if kind == tool.KindRead {
			return "path not found"
		}
		return "not found"
	case tool.ErrorCodeProtectedPath:
		return "protected path"
	case tool.ErrorCodeInternalPath:
		return "internal path"
	case tool.ErrorCodeOutsideWorkspace:
		return "outside workspace"
	case tool.ErrorCodeNoProgress:
		return "no progress"
	case tool.ErrorCodeStaleContinuation:
		return "stale continuation"
	case tool.ErrorCodeConflict:
		return "conflict"
	case tool.ErrorCodePreexistingWorkspaceChange:
		return "workspace changed"
	case tool.ErrorCodeWorkspaceStateUnavailable:
		return "workspace state unavailable"
	case tool.ErrorCodeSandboxUnavailable:
		return "sandbox unavailable"
	case tool.ErrorCodeExecution:
		return "execution failed"
	case tool.ErrorCodeNetworkUnavailable:
		return "network unavailable"
	default:
		return strings.ReplaceAll(strings.TrimSpace(string(code)), "_", " ")
	}
}

// RenderHeader renders one tool header while keeping state grammar and visual
// width policy in the same presentation package.
func RenderHeader(header Header, width int) string {
	width = max(1, width)
	label := textview.Sanitize(header.Label)
	meta := strings.TrimSpace(header.Meta)
	metaText := ""
	if meta != "" {
		if header.State == HeaderRunning {
			metaText = " " + meta
		} else {
			metaText = tuistyle.GlyphSep + meta
		}
	}

	target := ""
	if strings.TrimSpace(header.Target) != "" {
		reserved := ansi.StringWidth(header.Glyph) + ansi.StringWidth(label) + ansi.StringWidth(metaText) + 1
		target = " " + FormatPathWidth(header.Target, max(6, width-reserved))
	}

	glyphStyle := tuistyle.SuccessStyle
	metaStyle := tuistyle.ToolSummaryStyle
	labelStyle := tuistyle.MutedStyle
	switch header.State {
	case HeaderRunning:
		glyphStyle = tuistyle.ToolStyle
		metaStyle = tuistyle.ToolStyle
	case HeaderDenied:
		glyphStyle = tuistyle.WarningStyle
		metaStyle = tuistyle.WarningStyle
	case HeaderFailure:
		glyphStyle = tuistyle.ErrorStyle
		metaStyle = tuistyle.ErrorStyle
	}
	return glyphStyle.Render(header.Glyph) + labelStyle.Render(label) + target + metaStyle.Render(metaText)
}
