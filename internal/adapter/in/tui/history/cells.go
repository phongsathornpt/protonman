package history

const (
	defaultHistoryWidth    = 80
	defaultHistoryMaxLines = 1000
)

// HistoryCellKind identifies the semantic role of one transcript cell.
type HistoryCellKind uint8

const (
	// HistoryCellUnknown is the invalid zero value.
	HistoryCellUnknown HistoryCellKind = iota
	HistoryCellUser
	HistoryCellAssistant
	HistoryCellTool
	HistoryCellSystem
	HistoryCellError
)

// String returns the human-readable spelling of a history cell kind.
func (k HistoryCellKind) String() string {
	switch k {
	case HistoryCellUser:
		return "user"
	case HistoryCellAssistant:
		return "assistant"
	case HistoryCellTool:
		return "tool"
	case HistoryCellSystem:
		return "system"
	case HistoryCellError:
		return "error"
	default:
		return "unknown"
	}
}

// HistoryCell is the renderable unit of TUI conversation history.
//
// Rich rendering and raw/copy-friendly output intentionally live behind the
// same abstraction so specialized cells can evolve without teaching the root
// Bubble Tea model about every presentation type.
type HistoryCell interface {
	Kind() HistoryCellKind
	Render() []string
	RawLines() []string
	LineCount() int
}

// widthHistoryCell is implemented by cells whose rich presentation can wrap
// to the current viewport. The compatibility methods on HistoryCell remain
// available to callers that do not have a terminal width.
type widthHistoryCell interface {
	RenderWidth(width int) []string
}

func renderHistoryCell(cell HistoryCell, width int) []string {
	if sized, ok := cell.(widthHistoryCell); ok {
		return sized.RenderWidth(width)
	}
	return cell.Render()
}

func historyCellLineCount(cell HistoryCell, width int) int {
	return len(renderHistoryCell(cell, width))
}
