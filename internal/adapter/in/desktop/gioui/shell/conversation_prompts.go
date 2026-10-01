//go:build desktop || desktop_gio

package shell

import (
	"fmt"
	"io"
	"strings"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Permission, question, and audit panels: the interactive request surfaces
// rendered inline in the transcript.
func (s *Shell) layoutPermissionAuditItem(gtx layout.Context, item desktopstate.TimelineItem) layout.Dimensions {
	isAllow := strings.HasPrefix(item.Status, "Allowed") || item.Status == "answered"
	isDeny := strings.HasPrefix(item.Status, "Denied") || item.Status == "declined"

	accentColor := s.theme.Colors.Primary
	icon := uikit.KindCheck
	if isAllow {
		accentColor = s.theme.Colors.OnSuccessContainer
		icon = uikit.KindCheck
	} else if isDeny {
		accentColor = s.theme.Colors.OnErrorContainer
		icon = uikit.KindClose
	}

	return uikit.Inset{Top: 4, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLow, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 6, Bottom: 6, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.LayoutActionIcon(gtx, icon, 14, accentColor)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, item.Title, textLabelMedium, font.SemiBold, s.theme.Colors.OnSurface, 1)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if strings.TrimSpace(item.Text) == "" {
							return layout.Dimensions{}
						}
						return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, item.Text, textBodySmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 1)
						})
					}),
				)
			})
		})
	})
}

func (s *Shell) layoutPermissionPanel(gtx layout.Context, state desktopstate.State, request desktopstate.PermissionRequest) layout.Dimensions {
	if s.permissionButtons[request.RequestID] == nil {
		s.permissionButtons[request.RequestID] = make(map[string]*widget.Clickable)
	}
	for _, option := range request.Options {
		if s.permissionButtons[request.RequestID][option.ID] == nil {
			s.permissionButtons[request.RequestID][option.ID] = new(widget.Clickable)
		}
	}

	for {
		evt, ok := gtx.Event(
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEscape},
		)
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			if e.Name == key.NameReturn {
				for _, opt := range request.Options {
					optLower := strings.ToLower(opt.ID)
					if strings.Contains(optLower, "once") || strings.Contains(optLower, "allow") {
						s.bind.ResolvePermission(request.RequestID, opt.ID)
						break
					}
				}
			} else if e.Name == key.NameEscape {
				for _, opt := range request.Options {
					optLower := strings.ToLower(opt.ID)
					if strings.Contains(optLower, "reject") || strings.Contains(optLower, "deny") {
						s.bind.ResolvePermission(request.RequestID, opt.ID)
						break
					}
				}
			}
		}
	}

	qIdx := 1
	qTotal := 0
	for _, p := range state.PermissionInbox {
		if p.SessionID == request.SessionID {
			qTotal++
			if p.RequestID == request.RequestID {
				qIdx = qTotal
			}
		}
	}

	isElevatedRisk := request.Risk == "destructive" || strings.Contains(strings.ToLower(request.Risk), "destructive") || strings.Contains(strings.ToLower(request.Risk), "high")
	containerBg := s.theme.Colors.SurfaceContainerHigh
	borderColor := s.theme.Colors.OutlineVariant
	borderWidth := 1
	if isElevatedRisk {
		containerBg = s.theme.Colors.WarningContainer
		borderColor = s.theme.Colors.OnErrorContainer
		borderWidth = 2
	}

	toolLabel := request.ToolName
	if toolLabel == "" {
		toolLabel = "tool"
	}
	displayCmd := request.Command
	if displayCmd == "" {
		displayCmd = request.Detail
	}

	return s.roundedBorderSurface(gtx, shapeMedium, containerBg, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 12, Bottom: 12, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "⚡ "+toolLabel, textLabelSmall, font.Bold, s.theme.Colors.OnPrimaryContainer, 1)
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, request.Title, textTitleSmall, font.SemiBold, s.theme.Colors.OnSurface, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if qTotal <= 1 {
										return layout.Dimensions{}
									}
									counterText := fmt.Sprintf("(%d of %d)", qIdx, qTotal)
									return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, counterText, textLabelSmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 1)
									})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if isElevatedRisk {
								return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.ErrorContainer, func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, "⚠️ Elevated Risk", textLabelSmall, font.Bold, s.theme.Colors.OnErrorContainer, 1)
									})
								})
							}
							return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Permission Required", textLabelSmall, font.Medium, s.theme.Colors.OnSurfaceVariant, 1)
								})
							})
						}),
					)
				}),
			}

			if displayCmd != "" {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 8, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedBorderSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLowest, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, displayCmd, textBodyMedium, font.Normal, s.theme.Colors.OnSurface, 6)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if s.permissionCopyButtons[request.RequestID] == nil {
											s.permissionCopyButtons[request.RequestID] = new(widget.Clickable)
										}
										copyBtn := s.permissionCopyButtons[request.RequestID]
										return s.layoutIconActionButton(gtx, copyBtn, uikit.KindCopy, "Copy command", func() {
											gtx.Execute(clipboard.WriteCmd{
												Type: "application/text",
												Data: io.NopCloser(strings.NewReader(displayCmd)),
											})
										})
									}),
								)
							})
						})
					})
				}))
			}

			if strings.TrimSpace(request.RawJSON) != "" {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.permissionRawClickable[request.RequestID] == nil {
						s.permissionRawClickable[request.RequestID] = new(widget.Clickable)
					}
					toggleBtn := s.permissionRawClickable[request.RequestID]
					if toggleBtn.Clicked(gtx) {
						s.permissionRawToggles[request.RequestID] = !s.permissionRawToggles[request.RequestID]
					}
					expanded := s.permissionRawToggles[request.RequestID]
					toggleLabel := "▸ Show raw parameters (JSON)"
					if expanded {
						toggleLabel = "▾ Hide raw parameters (JSON)"
					}

					return uikit.Inset{Top: 2, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, toggleBtn, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, toggleLabel, textLabelSmall, font.Medium, s.theme.Colors.OnSurfaceVariant, 1)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !expanded {
									return layout.Dimensions{}
								}
								return uikit.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.roundedBorderSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLowest, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, request.RawJSON, textBodySmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 16)
										})
									})
								})
							}),
						)
					})
				}))
			}

			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					options := permissionOptionChildren(s, request)
					if len(options) <= 3 {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx, options...)
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, options...)
				})
			}))

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func permissionOptionChildren(s *Shell, request desktopstate.PermissionRequest) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(request.Options))
	for _, option := range request.Options {
		option := option
		child := layout.Rigid
		if len(request.Options) <= 3 {
			child = func(widget layout.Widget) layout.FlexChild { return layout.Flexed(1, widget) }
		}
		btn := s.permissionButtons[request.RequestID][option.ID]
		optLower := strings.ToLower(option.ID)

		children = append(children, child(func(gtx layout.Context) layout.Dimensions {
			return uikit.UniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if strings.Contains(optLower, "reject") || strings.Contains(optLower, "deny") {
					label := option.Name + " (Esc)"
					return s.layoutDangerButton(gtx, btn, label, true, func() {
						s.bind.ResolvePermission(request.RequestID, option.ID)
					})
				}
				if strings.Contains(optLower, "session") || strings.Contains(optLower, "always") {
					return s.layoutButton(gtx, btn, option.Name, true, func() {
						s.bind.ResolvePermission(request.RequestID, option.ID)
					})
				}
				label := option.Name + " (↵)"
				return s.layoutPrimaryButton(gtx, btn, label, true, func() {
					s.bind.ResolvePermission(request.RequestID, option.ID)
				})
			})
		}))
	}
	return children
}

type questionInteractionState struct {
	selectedOptions map[string]bool
	customAnswer    widget.Editor
	submitButton    widget.Clickable
	declineButton   widget.Clickable
	optionButtons   map[string]*widget.Clickable
}

func (s *Shell) layoutQuestionPanel(gtx layout.Context, state desktopstate.State, request desktopstate.QuestionRequest) layout.Dimensions {
	if s.questionStates[request.RequestID] == nil {
		qs := &questionInteractionState{
			selectedOptions: make(map[string]bool),
			optionButtons:   make(map[string]*widget.Clickable),
		}
		for _, q := range request.Questions {
			if q.Recommended != "" {
				qs.selectedOptions[q.Recommended] = true
			}
		}
		s.questionStates[request.RequestID] = qs
	}
	qs := s.questionStates[request.RequestID]

	for {
		evt, ok := gtx.Event(
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: "1"}, key.Filter{Name: "2"}, key.Filter{Name: "3"},
			key.Filter{Name: "4"}, key.Filter{Name: "5"}, key.Filter{Name: "6"},
			key.Filter{Name: "7"}, key.Filter{Name: "8"}, key.Filter{Name: "9"},
		)
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			if e.Name == key.NameEscape {
				delete(s.questionStates, request.RequestID)
				if s.bind.ResolveQuestion != nil {
					s.bind.ResolveQuestion(request.RequestID, desktopstate.QuestionResponse{
						Status: "declined",
						Answer: "User declined to answer",
					})
				}
				break
			}
			if e.Name == key.NameReturn && !gtx.Focused(&qs.customAnswer) {
				hasAnswer := len(qs.selectedOptions) > 0 || strings.TrimSpace(qs.customAnswer.Text()) != ""
				if hasAnswer {
					var selected []string
					for opt, sel := range qs.selectedOptions {
						if sel {
							selected = append(selected, opt)
						}
					}
					custom := strings.TrimSpace(qs.customAnswer.Text())
					answer := strings.Join(selected, ", ")
					if custom != "" {
						if answer != "" {
							answer += "\n" + custom
						} else {
							answer = custom
						}
					}
					delete(s.questionStates, request.RequestID)
					if s.bind.ResolveQuestion != nil {
						s.bind.ResolveQuestion(request.RequestID, desktopstate.QuestionResponse{
							Status:          "answered",
							Answer:          answer,
							SelectedOptions: selected,
						})
					}
				}
				break
			}
			if !gtx.Focused(&qs.customAnswer) && len(e.Name) == 1 && e.Name[0] >= '1' && e.Name[0] <= '9' {
				numIdx := int(e.Name[0] - '1')
				for _, q := range request.Questions {
					if numIdx < len(q.Options) {
						opt := q.Options[numIdx]
						if !q.Multiple {
							for _, otherOpt := range q.Options {
								delete(qs.selectedOptions, otherOpt)
							}
							qs.selectedOptions[opt] = true
						} else {
							if qs.selectedOptions[opt] {
								delete(qs.selectedOptions, opt)
							} else {
								qs.selectedOptions[opt] = true
							}
						}
					}
				}
			}
		}
	}

	for _, q := range request.Questions {
		for _, opt := range q.Options {
			if qs.optionButtons[opt] == nil {
				qs.optionButtons[opt] = new(widget.Clickable)
			}
			if qs.optionButtons[opt].Clicked(gtx) {
				if !q.Multiple {
					for _, otherOpt := range q.Options {
						delete(qs.selectedOptions, otherOpt)
					}
					qs.selectedOptions[opt] = true
				} else {
					if qs.selectedOptions[opt] {
						delete(qs.selectedOptions, opt)
					} else {
						qs.selectedOptions[opt] = true
					}
				}
			}
		}
	}

	return s.roundedSurface(gtx, shapeMedium, s.theme.Colors.SurfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 14, Bottom: 14, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "💬 Question from Agent", textTitleMedium, font.SemiBold, s.theme.Colors.Primary, 1)
						}),
					)
				}),
			}

			for _, q := range request.Questions {
				q := q
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 6, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, q.Question, textBodyMedium, font.Medium, s.theme.Colors.OnSurface, 4)
						})
					}),
				)
				if len(q.Options) > 0 {
					children = append(children,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							options := make([]layout.FlexChild, 0, len(q.Options))
							for optIdx, opt := range q.Options {
								optIdx := optIdx
								opt := opt
								selected := qs.selectedOptions[opt]
								btn := qs.optionButtons[opt]
								isRecommended := opt == q.Recommended

								child := layout.Rigid
								if len(q.Options) <= 2 {
									child = func(w layout.Widget) layout.FlexChild { return layout.Flexed(1, w) }
								}

								glyph := "( ) "
								if !q.Multiple {
									if selected {
										glyph = "(•) "
									}
								} else {
									glyph = "[ ] "
									if selected {
										glyph = "[✓] "
									}
								}
								hotkeyHint := fmt.Sprintf("[%d] ", optIdx+1)

								options = append(options, child(func(gtx layout.Context) layout.Dimensions {
									return uikit.UniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										bg := s.theme.Colors.SurfaceContainerLow
										fg := s.theme.Colors.OnSurface
										borderCol := s.theme.Colors.OutlineVariant
										if selected {
											bg = s.theme.Colors.PrimaryContainer
											fg = s.theme.Colors.OnPrimaryContainer
											borderCol = s.theme.Colors.Primary
										}
										return s.roundedBorderSurface(gtx, shapeSmall, bg, borderCol, 1, func(gtx layout.Context) layout.Dimensions {
											return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
												return uikit.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
														layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															label := glyph + hotkeyHint + opt
															return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, fg, 2)
														}),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															if !isRecommended {
																return layout.Dimensions{}
															}
															return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
																	return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																		return s.layoutLabel(gtx, "★ Recommended", textLabelSmall, font.SemiBold, s.theme.Colors.OnPrimaryContainer, 1)
																	})
																})
															})
														}),
													)
												})
											})
										})
									})
								}))
							}
							if len(q.Options) <= 2 {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, options...)
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, options...)
						}),
					)
				}
			}

			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 6, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedBorderSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLowest, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								ed := material.Editor(s.theme.Material, &qs.customAnswer, "Or type custom answer here…")
								ed.TextSize = unit.Sp(13)
								return ed.Layout(gtx)
							})
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutButton(gtx, &qs.declineButton, "Decline (Esc)", true, func() {
									delete(s.questionStates, request.RequestID)
									if s.bind.ResolveQuestion != nil {
										s.bind.ResolveQuestion(request.RequestID, desktopstate.QuestionResponse{
											Status: "declined",
											Answer: "User declined to answer",
										})
									}
								})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							hasAnswer := len(qs.selectedOptions) > 0 || strings.TrimSpace(qs.customAnswer.Text()) != ""
							return s.layoutPrimaryButton(gtx, &qs.submitButton, "Submit (↵)", hasAnswer, func() {
								var selected []string
								for opt, sel := range qs.selectedOptions {
									if sel {
										selected = append(selected, opt)
									}
								}
								custom := strings.TrimSpace(qs.customAnswer.Text())
								answer := strings.Join(selected, ", ")
								if custom != "" {
									if answer != "" {
										answer += "\n" + custom
									} else {
										answer = custom
									}
								}
								delete(s.questionStates, request.RequestID)
								if s.bind.ResolveQuestion != nil {
									s.bind.ResolveQuestion(request.RequestID, desktopstate.QuestionResponse{
										Status:          "answered",
										Answer:          answer,
										SelectedOptions: selected,
									})
								}
							})
						}),
					)
				}),
			)

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func activePermission(state desktopstate.State, sessionID string, agentIDs ...string) *desktopstate.PermissionRequest {
	for _, request := range state.PermissionInbox {
		if request.SessionID == sessionID && (len(agentIDs) == 0 || agentIDs[0] == "" || request.AgentID == agentIDs[0]) {
			item := request
			item.Options = append([]desktopstate.PermissionOption(nil), request.Options...)
			return &item
		}
	}
	return nil
}

func activeQuestion(state desktopstate.State, sessionID string, agentIDs ...string) *desktopstate.QuestionRequest {
	for _, request := range state.QuestionInbox {
		if request.SessionID == sessionID && (len(agentIDs) == 0 || agentIDs[0] == "" || request.AgentID == agentIDs[0]) {
			item := request
			return &item
		}
	}
	return nil
}
