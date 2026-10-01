//go:build desktop || desktop_gio

package shell

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

// baseChrome publishes the theme palette and the shell's rendering primitives
// exactly once. Every component package embeds this same surface, so a new
// primitive or token lands here instead of being re-wired per package.
func (s *Shell) baseChrome() uikit.Chrome {
	return uikit.Chrome{
		Colors:   s.theme.Colors,
		Material: s.theme.Material,

		Label:          s.layoutLabel,
		ActionIcon:     s.layoutIcon,
		RoundedSurface: s.roundedSurface,
		BorderSurface:  s.roundedBorderSurface,
		Divider:        s.layoutHorizontalDivider,
		Inset: func(gtx layout.Context, in layout.Inset, content layout.Widget) layout.Dimensions {
			return uikit.Inset(in).Layout(gtx, content)
		},
		Button:        s.layoutButton,
		PrimaryButton: s.layoutPrimaryButton,
		DangerButton:  s.layoutDangerButton,
		CloseButton: func(gtx layout.Context, button *widget.Clickable, close func()) layout.Dimensions {
			return s.layoutMiniIconButton(gtx, button, "✕", s.theme.Colors.OnSurfaceVariant)
		},
		PanelTitle:     s.layoutPanelTitle,
		MiniIconButton: s.layoutMiniIconButton,
		Editor:         s.layoutInspectorEditor,
		StatusDot:      s.layoutStatusDot,
		SettingsIcon: func(gtx layout.Context, tint color.NRGBA) layout.Dimensions {
			return uikit.LayoutActionIcon(gtx, uikit.KindSettings, 18, tint)
		},
		MonoFont: s.theme.TextFont,
	}
}

// layoutIcon resolves a component-requested icon through the shell's icon
// table. An unknown name falls back to a neutral glyph rather than dropping the
// element, so a new component icon never renders an empty frame.
func (s *Shell) layoutIcon(gtx layout.Context, icon uikit.Icon, size unit.Dp, tint color.NRGBA) layout.Dimensions {
	return uikit.LayoutActionIcon(gtx, uikit.ResolveIconKind(icon), size, tint)
}
