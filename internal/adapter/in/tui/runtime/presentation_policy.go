package runtime

import (
	presentationpolicy "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/presentationpolicy"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	"image/color"
)

type OpenCodeErrorKind = presentationpolicy.ErrorKind

type ClassifiedError = presentationpolicy.ClassifiedError

const (
	ErrorKindModelNotFound    = presentationpolicy.ErrorKind("model_not_found")
	ErrorKindContextOverflow  = presentationpolicy.ErrorKind("context_overflow")
	ErrorKindAuthentication   = presentationpolicy.ErrorKind("authentication")
	ErrorKindForbidden        = presentationpolicy.ErrorKind("forbidden")
	ErrorKindRateLimit        = presentationpolicy.ErrorKind("rate_limit")
	ErrorKindQuotaExceeded    = presentationpolicy.ErrorKind("quota_exceeded")
	ErrorKindServerOverloaded = presentationpolicy.ErrorKind("server_overloaded")
	ErrorKindStreamTimeout    = presentationpolicy.ErrorKind("stream_timeout")
	ErrorKindInvalidPrompt    = presentationpolicy.ErrorKind("invalid_prompt")
	ErrorKindMCPFailed        = presentationpolicy.ErrorKind("mcp_failed")
	ErrorKindConfigInvalid    = presentationpolicy.ErrorKind("config_invalid")
	ErrorKindConfigTypo       = presentationpolicy.ErrorKind("config_typo")
	ErrorKindToolFailed       = presentationpolicy.ErrorKind("tool_failed")
	ErrorKindToolDispatch     = presentationpolicy.ErrorKind("tool_dispatch")
	ErrorKindPermissionDenied = presentationpolicy.ErrorKind("permission_denied")
	ErrorKindCancelled        = presentationpolicy.ErrorKind("cancelled")
	ErrorKindGeneric          = presentationpolicy.ErrorKind("generic")
)

func ClassifyOpenCodeError(err error, activeProvider, activeModel string) ClassifiedError {
	return presentationpolicy.ClassifyError(err, activeProvider, activeModel)
}

func FormatErrorSummary(classified ClassifiedError) string {
	return presentationpolicy.FormatErrorSummary(classified)
}

type terminalLayoutMode = panecommon.LayoutMode

const (
	layoutNormal  = panecommon.LayoutNormal
	layoutCompact = panecommon.LayoutCompact
	layoutTiny    = panecommon.LayoutTiny
)

func layoutModeForHeight(height int) terminalLayoutMode {
	return presentationpolicy.LayoutModeForHeight(height)
}

func compactPickerRows(rows []string) []string {
	return presentationpolicy.CompactPickerRows(rows)
}

func renderModalRows(m *bubbleModel, border color.Color, rows []string) string {
	if m == nil {
		return panecommon.RenderModal(defaultBubbleWidth, defaultBubbleHeight, border, rows)
	}
	return panecommon.RenderModal(m.layout.width, m.layout.height, border, rows)
}

func paneToneColor(tone panecommon.Tone) color.Color {
	switch tone {
	case panecommon.ToneUser:
		return accentUser
	case panecommon.ToneError:
		return accentError
	case panecommon.ToneWarning:
		return warningColor
	default:
		return accentAssistant
	}
}
