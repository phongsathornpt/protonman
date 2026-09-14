package common

import (
	"image/color"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

// ToneColor resolves presentation intent through the semantic visual system.
// Pane renderers should depend on tone rather than raw palette values.
func ToneColor(tone Tone) color.Color {
	switch tone {
	case ToneUser:
		return tuistyle.AccentUser
	case ToneError:
		return tuistyle.ColorDanger
	case ToneWarning:
		return tuistyle.ColorWarning
	default:
		return tuistyle.ColorBorderFocus
	}
}

// RenderToneModal is the semantic modal entrypoint for tone-aware panes.
func RenderToneModal(width, height int, tone Tone, rows []string) string {
	return RenderModal(width, height, ToneColor(tone), rows)
}
