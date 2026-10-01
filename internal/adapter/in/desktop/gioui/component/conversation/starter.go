//go:build desktop || desktop_gio

package conversation

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
)

type StarterPrompt struct {
	Icon        StarterIcon
	Title       string
	Description string
	Prompt      string
}

type StarterIcon uint8

const (
	StarterSearch StarterIcon = iota
	StarterFolder
	StarterTerminal
	StarterCompose
)

var starterPrompts = [...]StarterPrompt{
	{Icon: StarterSearch, Title: "Explain codebase", Description: "Understand architecture & workflows", Prompt: "Explain the architecture, structure, and main workflows of this project."},
	{Icon: StarterFolder, Title: "Find & fix bugs", Description: "Inspect changes & resolve edge cases", Prompt: "Inspect recent changes, find any bugs or edge cases, and propose fixes."},
	{Icon: StarterTerminal, Title: "Run tests & verify", Description: "Execute test suite & analyze results", Prompt: "Run standard repository verification tests and report any failures."},
	{Icon: StarterCompose, Title: "Refactor & optimize", Description: "Improve code clarity & performance", Prompt: "Help me refactor and optimize code for clarity, performance, and maintainability."},
}

func StarterPrompts() []StarterPrompt { return starterPrompts[:] }

type StarterChrome struct {
	SurfaceContainerHigh, SurfaceContainerHighest, OutlineVariant color.NRGBA
	Primary, OnSurface, OnSurfaceVariant                          color.NRGBA
	BorderSurface                                                 func(layout.Context, unit.Dp, color.NRGBA, color.NRGBA, int, layout.Widget) layout.Dimensions
	Label                                                         func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions
	Icon                                                          func(layout.Context, StarterIcon, unit.Dp, color.NRGBA) layout.Dimensions
}

func (c *Component) LayoutStarterCard(gtx layout.Context, index int, chrome StarterChrome, onSelect func(string)) layout.Dimensions {
	if index < 0 || index >= len(starterPrompts) {
		return layout.Dimensions{}
	}
	prompt := starterPrompts[index]
	button := c.StarterButton(index)
	if button.Clicked(gtx) && onSelect != nil {
		onSelect(prompt.Prompt)
	}
	background, border := chrome.SurfaceContainerHigh, chrome.OutlineVariant
	if button.Hovered() {
		background, border = chrome.SurfaceContainerHighest, chrome.Primary
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(8), background, border, 1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return chrome.Icon(gtx, prompt.Icon, 20, chrome.Primary) })
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, prompt.Title, unit.Sp(14), font.SemiBold, chrome.OnSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, prompt.Description, unit.Sp(12), font.Normal, chrome.OnSurfaceVariant, 2)
								})
							}),
						)
					}),
				)
			})
		})
	})
}
