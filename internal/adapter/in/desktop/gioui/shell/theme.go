//go:build desktop || desktop_gio

package shell

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

// theme is the shell's resolved palette. The type itself lives in
// component/uikit so the desktop colour vocabulary has exactly one home; this
// alias lets the shell keep reading `s.theme.Colors.Surface` without
// re-declaring the token set alongside the one components consume.
// Theme is the resolved desktop palette. The token definitions live in
// component/uikit so the colour vocabulary has exactly one home.
type Theme = uikit.Theme

// theme is the in-package spelling used across the view files.
type theme = Theme

// NewTheme resolves the requested mode into a Theme.
func NewTheme(mode string) *Theme {
	return uikit.NewTheme(mode)
}

// Design tokens live in component/uikit so the shell and every component
// package share one vocabulary. These aliases keep the existing call sites
// readable without re-declaring the values.
const (
	shapeNone       = uikit.ShapeNone
	shapeSmall      = uikit.ShapeSmall
	shapeMedium     = uikit.ShapeMedium
	shapeLarge      = uikit.ShapeLarge
	shapeExtraLarge = uikit.ShapeExtraLarge
	shapeFull       = uikit.ShapeFull

	textDisplaySmall  = uikit.TextDisplaySmall
	textHeadlineSmall = uikit.TextHeadlineSmall
	textTitleMedium   = uikit.TextTitleMedium
	textTitleSmall    = uikit.TextTitleSmall
	textBodyLarge     = uikit.TextBodyLarge
	textBodyMedium    = uikit.TextBodyMedium
	textBodySmall     = uikit.TextBodySmall
	textLabelLarge    = uikit.TextLabelLarge
	textLabelMedium   = uikit.TextLabelMedium
	textLabelSmall    = uikit.TextLabelSmall
)
