package icon

import (
	"fmt"
	"strings"
)

// Mode selects the glyph capability profile used by the TUI.
type Mode string

const (
	ModeAuto    Mode = "auto"
	ModeNerd    Mode = "nerd"
	ModeUnicode Mode = "unicode"
	ModeASCII   Mode = "ascii"
)

// ParseMode validates a configured icon mode.
func ParseMode(value string) (Mode, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(value)))
	if mode == "" {
		return ModeAuto, nil
	}
	switch mode {
	case ModeAuto, ModeNerd, ModeUnicode, ModeASCII:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported icon mode %q", value)
	}
}

// Resolve returns a deterministic icon set. Auto deliberately does not guess
// the terminal font: font selection lives on the terminal side and is not
// reliably observable by the process, especially over SSH or tmux.
func Resolve(mode Mode, interactive bool) Set {
	switch mode {
	case ModeNerd:
		return Nerd
	case ModeASCII:
		return ASCII
	case ModeUnicode:
		return Unicode
	case ModeAuto, "":
		if !interactive {
			return ASCII
		}
		return Unicode
	default:
		return Unicode
	}
}
