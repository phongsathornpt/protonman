package runtime

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/diagnostic"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	"image/color"
)

type OpenCodeErrorKind = diagnostic.Kind

type ClassifiedError = diagnostic.Error

const (
	ErrorKindModelNotFound    = diagnostic.KindModelNotFound
	ErrorKindContextOverflow  = diagnostic.KindContextOverflow
	ErrorKindAuthentication   = diagnostic.KindAuthentication
	ErrorKindForbidden        = diagnostic.KindForbidden
	ErrorKindRateLimit        = diagnostic.KindRateLimit
	ErrorKindQuotaExceeded    = diagnostic.KindQuotaExceeded
	ErrorKindServerOverloaded = diagnostic.KindServerOverloaded
	ErrorKindStreamTimeout    = diagnostic.KindStreamTimeout
	ErrorKindInvalidPrompt    = diagnostic.KindInvalidPrompt
	ErrorKindMCPFailed        = diagnostic.KindMCPFailed
	ErrorKindConfigInvalid    = diagnostic.KindConfigInvalid
	ErrorKindConfigTypo       = diagnostic.KindConfigTypo
	ErrorKindToolFailed       = diagnostic.KindToolFailed
	ErrorKindToolDispatch     = diagnostic.KindToolDispatch
	ErrorKindPermissionDenied = diagnostic.KindPermissionDenied
	ErrorKindCancelled        = diagnostic.KindCancelled
	ErrorKindGeneric          = diagnostic.KindGeneric
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

func pickerVisibleRows(height, maximum int) int {
	return panecommon.PickerVisibleRows(height, maximum)
}

func compactPickerRows(rows []string) []string {
	return panecommon.CompactRows(rows)
}

func renderModalRows(m *bubbleModel, border color.Color, rows []string) string {
	if m == nil {
		return panecommon.RenderModal(defaultBubbleWidth, defaultBubbleHeight, border, rows)
	}
	return panecommon.RenderModal(m.width, m.height, border, rows)
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
