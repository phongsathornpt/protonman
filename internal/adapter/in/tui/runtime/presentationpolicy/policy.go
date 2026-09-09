package presentationpolicy

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/diagnostic"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
)

type ErrorKind = diagnostic.Kind
type ClassifiedError = diagnostic.Error
type LayoutMode = panecommon.LayoutMode

func ClassifyError(err error, provider, model string) ClassifiedError {
	return diagnostic.Classify(err, provider, model)
}

func FormatErrorSummary(classified ClassifiedError) string {
	return diagnostic.FormatSummary(classified)
}

func LayoutModeForHeight(height int) LayoutMode {
	return panecommon.ModeForHeight(height)
}

func PickerVisibleRows(height, maximum int) int {
	return panecommon.PickerVisibleRows(height, maximum)
}

func CompactPickerRows(rows []string) []string {
	return panecommon.CompactRows(rows)
}
