//go:build desktop || desktop_gio

package shell

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// The empty-session placeholder and its starter prompt cards.
func (s *Shell) layoutEmptyState(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	wsName := "Workspace"
	if session.Workspace != "" {
		wsName = filepath.Base(session.Workspace)
	}

	modelName := session.Runtime.Model
	if strings.TrimSpace(modelName) == "" {
		modelName = "auto"
	}

	agentName := "Protonman"
	if strings.TrimSpace(session.AgentID) != "" {
		agentName = session.AgentID
	}

	if gtx.Constraints.Max.X > 0 {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(660))
		return uikit.Inset{Top: 24, Bottom: 16, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Protonman", textHeadlineSmall, font.Bold, s.theme.Colors.OnSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Top: 4, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Autonomous coding agent for "+wsName, textBodyMedium, font.Normal, s.theme.Colors.OnSurfaceVariant, 1)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "Agent: "+agentName, textLabelSmall, font.SemiBold, s.theme.Colors.OnPrimaryContainer, 1)
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerHighest, func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, "Model: "+modelName, textLabelSmall, font.Medium, s.theme.Colors.OnSurface, 1)
											})
										})
									})
								}),
							)
						}),
					)
				}),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 20, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						twoCol := gtx.Constraints.Max.X >= gtx.Dp(460)
						if twoCol {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											return s.layoutStarterCard(gtx, 0)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return layout.Spacer{Width: 10}.Layout(gtx)
										}),
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											return s.layoutStarterCard(gtx, 1)
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												return s.layoutStarterCard(gtx, 2)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Spacer{Width: 10}.Layout(gtx)
											}),
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												return s.layoutStarterCard(gtx, 3)
											}),
										)
									})
								}),
							)
						}

						children := make([]layout.FlexChild, 0, 4)
						for i := 0; i < 4; i++ {
							idx := i
							var topInset unit.Dp
							if idx > 0 {
								topInset = 8
							}
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Top: topInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutStarterCard(gtx, idx)
								})
							}))
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
					})
				}),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.roundedSurface(gtx, shapeFull, s.theme.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "@ mention files  ·  / commands  ·  Alt+M select model  ·  Cmd+N new chat", textLabelSmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 1)
						})
					})
				}),
			)
		})
	}))
}

func (s *Shell) layoutStarterCard(gtx layout.Context, index int) layout.Dimensions {
	return s.conversationUI.LayoutStarterCard(gtx, index, conversationcomponent.StarterChrome{
		SurfaceContainerHigh: s.theme.Colors.SurfaceContainerHigh, SurfaceContainerHighest: s.theme.Colors.SurfaceContainerHighest,
		OutlineVariant: s.theme.Colors.OutlineVariant, Primary: s.theme.Colors.Primary,
		OnSurface: s.theme.Colors.OnSurface, OnSurfaceVariant: s.theme.Colors.OnSurfaceVariant,
		BorderSurface: s.roundedBorderSurface, Label: s.layoutLabel,
		Icon: func(gtx layout.Context, icon conversationcomponent.StarterIcon, size unit.Dp, tint color.NRGBA) layout.Dimensions {
			kind := uikit.KindSearch
			switch icon {
			case conversationcomponent.StarterFolder:
				kind = uikit.KindFolder
			case conversationcomponent.StarterTerminal:
				kind = uikit.KindTerminal
			case conversationcomponent.StarterCompose:
				kind = uikit.KindCompose
			}
			return uikit.LayoutActionIcon(gtx, kind, size, tint)
		},
	}, func(text string) {
		s.setComposerText(text)
		s.conversationUI.Editor().SetCaret(len(text), len(text))
		s.composerError = ""
		s.conversationUI.Timeline().ScrollToEnd = true
		s.conversationUI.Timeline().Position = layout.Position{}
		gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
	})
}
