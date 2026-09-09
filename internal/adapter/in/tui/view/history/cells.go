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
	RenderWidth(width int) []string
	RawLines() []string
	LineCount() int
}

func renderHistoryCell(cell HistoryCell, width int) []string {
	return cell.RenderWidth(width)
}

func historyCellLineCount(cell HistoryCell, width int) int {
	return len(renderHistoryCell(cell, width))
}
