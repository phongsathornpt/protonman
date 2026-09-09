package runtime

import (
	"image/color"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/diagnostic"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
)

type OpenCodeErrorKind = diagnostic.Kind

type ClassifiedError = diagnostic.Error

const (
	ErrorKindModelNotFound    = diagnostic.Kind("model_not_found")
	ErrorKindContextOverflow  = diagnostic.Kind("context_overflow")
	ErrorKindAuthentication   = diagnostic.Kind("authentication")
	ErrorKindForbidden        = diagnostic.Kind("forbidden")
	ErrorKindRateLimit        = diagnostic.Kind("rate_limit")
	ErrorKindQuotaExceeded    = diagnostic.Kind("quota_exceeded")
	ErrorKindServerOverloaded = diagnostic.Kind("server_overloaded")
	ErrorKindStreamTimeout    = diagnostic.Kind("stream_timeout")
	ErrorKindInvalidPrompt    = diagnostic.Kind("invalid_prompt")
	ErrorKindMCPFailed        = diagnostic.Kind("mcp_failed")
	ErrorKindConfigInvalid    = diagnostic.Kind("config_invalid")
	ErrorKindConfigTypo       = diagnostic.Kind("config_typo")
	ErrorKindToolFailed       = diagnostic.Kind("tool_failed")
	ErrorKindToolDispatch     = diagnostic.Kind("tool_dispatch")
	ErrorKindPermissionDenied = diagnostic.Kind("permission_denied")
	ErrorKindCancelled        = diagnostic.Kind("cancelled")
	ErrorKindGeneric          = diagnostic.Kind("generic")
)

func ClassifyOpenCodeError(err error, activeProvider, activeModel string) ClassifiedError {
	return diagnostic.Classify(err, activeProvider, activeModel)
}

func FormatErrorSummary(classified ClassifiedError) string {
	return diagnostic.FormatSummary(classified)
}

type terminalLayoutMode = panecommon.LayoutMode

const (
	layoutNormal  = panecommon.LayoutNormal
	layoutCompact = panecommon.LayoutCompact
	layoutTiny    = panecommon.LayoutTiny
)

func layoutModeForHeight(height int) terminalLayoutMode {
	return panecommon.ModeForHeight(height)
}

func compactPickerRows(rows []string) []string {
	return panecommon.CompactRows(rows)
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
