package runtime

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func testKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func testText(text string) tea.KeyPressMsg {
	runes := []rune(text)
	var code rune
	if len(runes) == 1 {
		code = runes[0]
	}
	return tea.KeyPressMsg{Code: code, Text: text}
}

func testCtrl(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl}
}

func testAltText(text string) tea.KeyPressMsg {
	runes := []rune(text)
	var code rune
	if len(runes) > 0 {
		code = runes[0]
	}
	return tea.KeyPressMsg{Code: code, Mod: tea.ModAlt}
}
func testShiftTab() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
}

func testPlain(text string) string {
	return ansi.Strip(text)
}
