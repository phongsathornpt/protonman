//go:build desktop || desktop_gio

package conversation

import (
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
)

type MentionItemKind uint8

const (
	MentionItemKindAgent MentionItemKind = iota
	MentionItemKindFile
	MentionItemKindDir
)

type MentionItem struct {
	Kind        MentionItemKind
	Name        string
	Description string
	PrefixTag   string
	LineCount   int
	SizeBytes   int64
}

func (i MentionItem) Title() string {
	prefix := "@"
	if i.PrefixTag != "" {
		prefix = i.PrefixTag + " @"
	}
	if i.Kind == MentionItemKindDir && !strings.HasSuffix(i.Name, "/") {
		return prefix + i.Name + "/"
	}
	return prefix + i.Name
}

func (i MentionItem) InsertionText() string {
	if i.Kind == MentionItemKindDir {
		if strings.HasSuffix(i.Name, "/") {
			return "@" + i.Name
		}
		return "@" + i.Name + "/"
	}
	return "@" + i.Name + " "
}

type MentionChrome struct {
	OnSurface, OnSurfaceVariant, OnPrimaryContainer, OnSecondaryContainer color.NRGBA
	PrimaryContainer, SecondaryContainer, SurfaceContainer                color.NRGBA
	SurfaceContainerHigh, SurfaceContainerHighest, OutlineVariant         color.NRGBA
	Label                                                                 func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions
	RoundedSurface                                                        func(layout.Context, unit.Dp, color.NRGBA, layout.Widget) layout.Dimensions
	BorderSurface                                                         func(layout.Context, unit.Dp, color.NRGBA, color.NRGBA, int, layout.Widget) layout.Dimensions
}

func (c *Component) LayoutMentions(gtx layout.Context, items []MentionItem, selected int, chrome MentionChrome, onSelect func(MentionItem)) layout.Dimensions {
	if len(items) == 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints.Max.Y = gtx.Dp(220)
	return layout.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(12), chrome.SurfaceContainerHigh, chrome.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 4, Bottom: 4, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				c.mentionList.Axis = layout.Vertical
				return c.mentionList.Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
					item := items[index]
					isSelected := index == selected
					button := c.MentionButton(item.Name)
					if button.Clicked(gtx) && onSelect != nil {
						onSelect(item)
					}
					background := chrome.SurfaceContainerHigh
					if isSelected || button.Hovered() {
						background = chrome.PrimaryContainer
					}
					return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.RoundedSurface(gtx, unit.Dp(6), background, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										tagBackground, tagForeground := chrome.SurfaceContainer, chrome.OnSurfaceVariant
										switch item.Kind {
										case MentionItemKindAgent:
											tagBackground, tagForeground = chrome.PrimaryContainer, chrome.OnPrimaryContainer
										case MentionItemKindFile:
											tagBackground, tagForeground = chrome.SecondaryContainer, chrome.OnSecondaryContainer
										case MentionItemKindDir:
											tagBackground = chrome.SurfaceContainerHighest
										}
										return layout.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return chrome.RoundedSurface(gtx, unit.Dp(6), tagBackground, func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return chrome.Label(gtx, item.PrefixTag, unit.Sp(11), font.Medium, tagForeground, 1)
												})
											})
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										displayName := item.Name
										if item.Kind == MentionItemKindDir && !strings.HasSuffix(displayName, "/") {
											displayName += "/"
										}
										foreground := chrome.OnSurface
										if isSelected {
											foreground = chrome.OnPrimaryContainer
										}
										return chrome.Label(gtx, "@"+displayName, unit.Sp(14), font.Medium, foreground, 1)
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										if item.Description == "" {
											return layout.Spacer{}.Layout(gtx)
										}
										return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return chrome.Label(gtx, item.Description, unit.Sp(11), font.Normal, chrome.OnSurfaceVariant, 1)
										})
									}),
								)
							})
						})
					})
				})
			})
		})
	})
}
