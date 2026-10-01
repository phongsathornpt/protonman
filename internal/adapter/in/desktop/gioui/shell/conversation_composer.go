//go:build desktop || desktop_gio

package shell

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// The composer: input editor, action buttons, context chips, and per-session
// draft persistence.
func (s *Shell) layoutComposer(gtx layout.Context, session desktopstate.SessionState, snapshot controller.Snapshot) layout.Dimensions {
	busy := controller.SessionBusy(session.Status)
	connected := sessionConnection(snapshot, session.AgentID) == controller.ConnectionConnected
	loading := snapshot.HistoryState == controller.HistoryStateLoading
	canSend := connected && !busy && !loading
	canEdit := canSend && !s.settingsComponent.IsOpen()
	compact := gtx.Constraints.Max.X < gtx.Dp(480)
	helper := composerHelper(compact, busy, loading, connected)
	s.activeWorkspace = session.Workspace
	s.conversationUI.Editor().ReadOnly = !canEdit
	if canEdit {
		s.refreshMentionState(session.Workspace)
		if s.mentionActive && len(s.mentionItems) > 0 {
			for {
				evt, ok := gtx.Event(key.Filter{Focus: s.conversationUI.Editor(), Name: key.NameUpArrow})
				if !ok {
					break
				}
				if e, ok := evt.(key.Event); ok && e.State == key.Press {
					s.mentionSelectedIndex--
					if s.mentionSelectedIndex < 0 {
						s.mentionSelectedIndex = len(s.mentionItems) - 1
					}
				}
			}
			for {
				evt, ok := gtx.Event(key.Filter{Focus: s.conversationUI.Editor(), Name: key.NameDownArrow})
				if !ok {
					break
				}
				if e, ok := evt.(key.Event); ok && e.State == key.Press {
					s.mentionSelectedIndex++
					if s.mentionSelectedIndex >= len(s.mentionItems) {
						s.mentionSelectedIndex = 0
					}
				}
			}
			for {
				evt, ok := gtx.Event(key.Filter{Focus: s.conversationUI.Editor(), Name: key.NameTab})
				if !ok {
					break
				}
				if e, ok := evt.(key.Event); ok && e.State == key.Press {
					s.applySelectedMention()
				}
			}
			for {
				evt, ok := gtx.Event(key.Filter{Focus: s.conversationUI.Editor(), Name: key.NameEscape})
				if !ok {
					break
				}
				if e, ok := evt.(key.Event); ok && e.State == key.Press {
					s.mentionDismissed = true
					s.mentionDismissedQuery = s.mentionContext.Query
					s.mentionActive = false
				}
			}
		}
		for {
			event, ok := s.conversationUI.Editor().Update(gtx)
			if !ok {
				break
			}
			switch event.(type) {
			case widget.ChangeEvent:
				s.composerError = ""
				s.mentionStateDirty = true
				s.refreshMentionState(session.Workspace)
			case widget.SelectEvent:
				s.mentionStateDirty = true
				s.refreshMentionState(session.Workspace)
			}
			if submit, ok := event.(widget.SubmitEvent); ok {
				if s.mentionActive && len(s.mentionItems) > 0 {
					s.applySelectedMention()
				} else {
					s.submitComposer(submit.Text)
				}
			}
		}
	}

	outerInset := uikit.Inset{Top: 6, Bottom: 16, Left: 16, Right: 16}
	if compact {
		outerInset = uikit.Inset{Top: 4, Bottom: 8, Left: 8, Right: 8}
	}

	helperColor := s.theme.Colors.OnSurfaceVariant
	if s.composerError != "" {
		helper = s.composerError
		helperColor = s.theme.Colors.OnErrorContainer
	}

	borderColor := s.theme.Colors.OutlineVariant
	borderWidth := 1
	if gtx.Focused(s.conversationUI.Editor()) {
		borderColor = s.theme.Colors.Primary
		borderWidth = 2
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(960))
		return outerInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			containerChildren := make([]layout.FlexChild, 0, 2)
			if s.mentionActive && len(s.mentionItems) > 0 {
				containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMentionPopup(gtx)
				}))
			} else if s.runtimeComponent.Widgets().ModelPopoverVisible {
				containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutModelPopover(gtx, session, snapshot, canSend)
				}))
			} else if s.runtimeComponent.Widgets().ReasoningPopoverVisible {
				containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutReasoningPopover(gtx, session, canSend)
				}))
			} else if s.runtimeComponent.Widgets().PermissionModePopoverVisible {
				containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutPermissionModePopover(gtx, session, canSend)
				}))
			}

			containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.roundedBorderSurface(gtx, shapeExtraLarge, s.theme.Colors.SurfaceContainer, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
					innerInset := uikit.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}
					if compact {
						innerInset = uikit.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}
					}
					return innerInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutComposerContextChips(gtx, session, snapshot, canSend)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutComposerEditor(gtx, canSend, compact)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											if helper == "" {
												return layout.Spacer{}.Layout(gtx)
											}
											return uikit.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, helper, textLabelSmall, font.Normal, helperColor, 1)
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if busy {
												return s.layoutComposerActionButton(gtx, s.conversationUI.StopButton(), "Stop", "Stop prompt", connected, true, s.bind.CancelPrompt)
											}
											label := "↑"
											if !compact {
												label = "Send"
											}
											return s.layoutComposerActionButton(gtx, s.conversationUI.SendButton(), label, "Send prompt", canSend && s.composerHasContent, false, func() {
												s.submitComposer(s.conversationUI.Editor().Text())
											})
										}),
									)
								})
							}),
						)
					})
				})
			}))

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, containerChildren...)
		})
	}))
}

func (s *Shell) layoutComposerActionButton(gtx layout.Context, button *widget.Clickable, icon, tooltip string, enabled, danger bool, action func()) layout.Dimensions {
	if enabled && button.Clicked(gtx) && action != nil {
		action()
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	size := gtx.Dp(32)
	width := size
	if icon == "Send" {
		width = gtx.Dp(76)
	}
	gtx.Constraints.Min = image.Pt(width, size)
	gtx.Constraints.Max = image.Pt(width, size)
	semantic.Button.Add(gtx.Ops)
	semantic.EnabledOp(gtx.Enabled()).Add(gtx.Ops)
	semantic.DescriptionOp(tooltip).Add(gtx.Ops)
	bg := s.theme.Colors.Primary
	fg := s.theme.Colors.OnPrimary
	if danger {
		bg = s.theme.Colors.ErrorContainer
		fg = s.theme.Colors.OnErrorContainer
	}
	if !gtx.Enabled() {
		bg = s.theme.Colors.SurfaceContainerHigh
		fg = s.theme.Colors.OnSurfaceVariant
	} else if button.Hovered() && !danger {
		bg = s.theme.Colors.PrimaryContainer
		fg = s.theme.Colors.OnPrimaryContainer
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeFull, bg, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				kind := uikit.KindSend
				if icon == "Stop" {
					kind = uikit.KindStop
				}
				if icon == "↑" || icon == "Stop" {
					return uikit.LayoutActionIcon(gtx, kind, 16, fg)
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return uikit.LayoutActionIcon(gtx, kind, 16, fg) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, icon, textTitleMedium, font.Bold, fg, 1)
						})
					}),
				)
			}))
		})
	})
	if enabled && gtx.Focused(button) {
		widget.Border{Color: s.theme.Colors.Primary, CornerRadius: shapeFull, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func composerHelper(compact, busy, loading, connected bool) string {
	switch {
	case busy:
		return "Use Stop to cancel the active turn"
	case loading:
		return "Prompting resumes after session history finishes loading"
	case !connected:
		return "Reconnect to the ACP runtime before sending"
	case compact:
		return ""
	default:
		return "Enter sends · Shift+Enter adds a line"
	}
}

func (s *Shell) layoutComposerContextChips(gtx layout.Context, session desktopstate.SessionState, snapshot controller.Snapshot, enabled bool) layout.Dimensions {
	chips := make([]layout.FlexChild, 0, 4)
	goalLimit, modelLimit := 36, 28
	reasoningLabel := "Reasoning: "
	modeLabel := "Mode: "
	if gtx.Constraints.Max.X < gtx.Dp(760) {
		goalLimit, modelLimit = 22, 18
		reasoningLabel = "Effort: "
		modeLabel = "Mode: "
	}
	if gtx.Constraints.Max.X < gtx.Dp(640) {
		goalLimit, modelLimit = 16, 14
		reasoningLabel = "Effort: "
		modeLabel = ""
	}
	if gtx.Constraints.Max.X < gtx.Dp(520) {
		goalLimit, modelLimit = 12, 12
		reasoningLabel = ""
		modeLabel = ""
	}
	if gtx.Constraints.Max.X < gtx.Dp(380) {
		goalLimit = 8
		modelLimit = 10
		reasoningLabel = ""
		modeLabel = ""
	}
	var contextDescription strings.Builder
	if goal := strings.TrimSpace(session.Context.Goal); goal != "" {
		contextDescription.WriteString("Goal: ")
		contextDescription.WriteString(goal)
	}
	agentID := strings.TrimSpace(session.AgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(snapshot.ActiveAgentID)
	}
	if agentID == "" {
		agentID = controller.ProtonmanAgentID
	}
	model := strings.TrimSpace(session.Runtime.Model)
	if model == "" && snapshot.AgentDefaultModels != nil {
		model = strings.TrimSpace(snapshot.AgentDefaultModels[agentID])
	}
	if model == "" {
		model = "default"
	}
	if contextDescription.Len() > 0 {
		contextDescription.WriteString(". ")
	}
	contextDescription.WriteString("Model: ")
	contextDescription.WriteString(model)

	reasoning := strings.TrimSpace(session.Runtime.Reasoning)
	if reasoning == "" {
		reasoning = "auto"
	}
	if contextDescription.Len() > 0 {
		contextDescription.WriteString(". ")
	}
	contextDescription.WriteString("Reasoning: ")
	contextDescription.WriteString(reasoning)

	mode := strings.TrimSpace(session.Runtime.PermissionMode)
	if mode == "" {
		mode = "ask"
	}
	displayMode := "Ask"
	switch mode {
	case "plan", "deny":
		displayMode = "Plan"
	case "always-approve", "auto":
		displayMode = "Always Approve"
	default:
		displayMode = strings.ToUpper(mode[:1]) + mode[1:]
	}
	if contextDescription.Len() > 0 {
		contextDescription.WriteString(". ")
	}
	contextDescription.WriteString("Mode: ")
	contextDescription.WriteString(displayMode)

	if contextDescription.Len() > 0 {
		semantic.DescriptionOp("Prompt context. " + contextDescription.String()).Add(gtx.Ops)
	}

	if goal := strings.TrimSpace(session.Context.Goal); goal != "" {
		chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeMedium, s.theme.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "Goal: "+compactInspectorText(goal, goalLimit), textLabelSmall, font.Medium, s.theme.Colors.OnPrimaryContainer, 1)
					})
				})
			})
		}))
	}

	chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if enabled && s.runtimeComponent.Widgets().ModelChipButton.Clicked(gtx) {
			if s.runtimeComponent.Widgets().ModelPopoverVisible {
				s.closePopovers()
				gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
			} else {
				s.openModelPopover()
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		semantic.DescriptionOp("Change model (Alt+M)").Add(gtx.Ops)
		bg := s.theme.Colors.SecondaryContainer
		fg := s.theme.Colors.OnSecondaryContainer
		if s.runtimeComponent.Widgets().ModelPopoverVisible {
			bg = s.theme.Colors.PrimaryContainer
			fg = s.theme.Colors.OnPrimaryContainer
		} else if enabled && s.runtimeComponent.Widgets().ModelChipButton.Hovered() {
			bg = s.theme.Colors.SurfaceContainerHighest
		}
		chipGtx := gtx
		if !enabled {
			chipGtx = chipGtx.Disabled()
		}
		chevronKind := uikit.KindChevronDown
		if s.runtimeComponent.Widgets().ModelPopoverVisible {
			chevronKind = uikit.KindChevronUp
		}
		return uikit.Inset{Right: 6}.Layout(chipGtx, func(gtx layout.Context) layout.Dimensions {
			return s.runtimeComponent.Widgets().ModelChipButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := s.roundedSurface(gtx, shapeMedium, bg, func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Model: "+compactInspectorText(model, modelLimit), textLabelSmall, font.Medium, fg, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return uikit.LayoutActionIcon(gtx, chevronKind, 10, fg)
								})
							}),
						)
					})
				})
				if s.runtimeComponent.Widgets().ModelPopoverVisible {
					widget.Border{Color: s.theme.Colors.Primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: dims.Size}
					})
				}
				return dims
			})
		})
	}))

	chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if enabled && s.runtimeComponent.Widgets().ReasoningChipButton.Clicked(gtx) {
			if s.runtimeComponent.Widgets().ReasoningPopoverVisible {
				s.closePopovers()
				gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
			} else {
				s.openReasoningPopover()
				gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		semantic.DescriptionOp("Change reasoning effort (Alt+R)").Add(gtx.Ops)
		bg := s.theme.Colors.TertiaryContainer
		fg := s.theme.Colors.OnTertiaryContainer
		if s.runtimeComponent.Widgets().ReasoningPopoverVisible {
			bg = s.theme.Colors.PrimaryContainer
			fg = s.theme.Colors.OnPrimaryContainer
		} else if enabled && s.runtimeComponent.Widgets().ReasoningChipButton.Hovered() {
			bg = s.theme.Colors.SurfaceContainerHighest
		}
		chipGtx := gtx
		if !enabled {
			chipGtx = chipGtx.Disabled()
		}
		chevronKind := uikit.KindChevronDown
		if s.runtimeComponent.Widgets().ReasoningPopoverVisible {
			chevronKind = uikit.KindChevronUp
		}
		return uikit.Inset{Right: 6}.Layout(chipGtx, func(gtx layout.Context) layout.Dimensions {
			return s.runtimeComponent.Widgets().ReasoningChipButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := s.roundedSurface(gtx, shapeMedium, bg, func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, reasoningLabel+reasoning, textLabelSmall, font.Medium, fg, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return uikit.LayoutActionIcon(gtx, chevronKind, 10, fg)
								})
							}),
						)
					})
				})
				if s.runtimeComponent.Widgets().ReasoningPopoverVisible {
					widget.Border{Color: s.theme.Colors.Primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: dims.Size}
					})
				}
				return dims
			})
		})
	}))

	chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if enabled && s.runtimeComponent.Widgets().PermissionModeChipButton.Clicked(gtx) {
			if s.runtimeComponent.Widgets().PermissionModePopoverVisible {
				s.closePopovers()
				gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
			} else {
				s.openPermissionModePopover()
				gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		semantic.DescriptionOp("Change permission mode (Alt+P)").Add(gtx.Ops)
		bg := s.theme.Colors.SurfaceContainerHigh
		fg := s.theme.Colors.OnSurface
		switch mode {
		case "plan", "deny":
			bg = s.theme.Colors.SecondaryContainer
			fg = s.theme.Colors.OnSecondaryContainer
		case "always-approve", "auto":
			bg = s.theme.Colors.WarningContainer
			fg = s.theme.Colors.OnWarningContainer
		}
		if s.runtimeComponent.Widgets().PermissionModePopoverVisible {
			bg = s.theme.Colors.PrimaryContainer
			fg = s.theme.Colors.OnPrimaryContainer
		} else if enabled && s.runtimeComponent.Widgets().PermissionModeChipButton.Hovered() {
			bg = s.theme.Colors.SurfaceContainerHighest
		}
		chipGtx := gtx
		if !enabled {
			chipGtx = chipGtx.Disabled()
		}
		chevronKind := uikit.KindChevronDown
		if s.runtimeComponent.Widgets().PermissionModePopoverVisible {
			chevronKind = uikit.KindChevronUp
		}
		return uikit.Inset{Right: 6}.Layout(chipGtx, func(gtx layout.Context) layout.Dimensions {
			return s.runtimeComponent.Widgets().PermissionModeChipButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := s.roundedSurface(gtx, shapeMedium, bg, func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, modeLabel+displayMode, textLabelSmall, font.Medium, fg, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return uikit.LayoutActionIcon(gtx, chevronKind, 10, fg)
								})
							}),
						)
					})
				})
				if s.runtimeComponent.Widgets().PermissionModePopoverVisible {
					widget.Border{Color: s.theme.Colors.Primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: dims.Size}
					})
				}
				return dims
			})
		})
	}))

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, chips...)
}

func (s *Shell) layoutComposerEditor(gtx layout.Context, enabled, compact bool) layout.Dimensions {
	minHeight, maxHeight, verticalInset := gtx.Dp(44), gtx.Dp(180), unit.Dp(8)
	if compact {
		minHeight, maxHeight, verticalInset = gtx.Dp(40), gtx.Dp(140), unit.Dp(6)
	}
	gtx.Constraints.Min.Y = minHeight
	gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, maxHeight)
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	textColor := s.theme.Colors.OnSurface
	if !enabled {
		textColor = s.theme.Colors.OnSurfaceVariant
	}
	textMaterial := op.Record(gtx.Ops)
	paint.ColorOp{Color: textColor}.Add(gtx.Ops)
	textCall := textMaterial.Stop()
	selectionMaterial := op.Record(gtx.Ops)
	paint.ColorOp{Color: s.theme.Colors.PrimaryContainer}.Add(gtx.Ops)
	selectionCall := selectionMaterial.Stop()
	return uikit.Inset{Top: verticalInset, Bottom: verticalInset, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.StackChild{
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				semantic.DescriptionOp("Message Protonman. Press Enter to send; Shift+Enter to insert a new line.").Add(gtx.Ops)
				return s.conversationUI.Editor().Layout(gtx, s.theme.Material.Shaper, s.theme.TextFont(font.Normal), textBodyMedium, textCall, selectionCall)
			}),
		}
		if strings.TrimSpace(s.conversationUI.Editor().Text()) == "" && !gtx.Focused(s.conversationUI.Editor()) {
			children = append(children, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Message Protonman…", textBodyMedium, font.Normal, s.theme.Colors.OnSurfaceVariant, 1)
			}))
		}
		return layout.Stack{Alignment: layout.NW}.Layout(gtx, children...)
	})
}

func (s *Shell) submitComposer(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	expanded, err := controller.ExpandMentions(text, s.activeWorkspace)
	if err != nil {
		s.composerError = err.Error()
		return
	}
	s.composerError = ""
	s.bind.SendPrompt(expanded)
	s.setComposerText("")
	s.forgetComposerDraft(controller.SessionRefStorageKey(desktopstate.SessionRef{AgentID: s.activeSessionAgentID, SessionID: s.activeSessionID}))
	s.conversationUI.Timeline().ScrollToEnd = true
	s.conversationUI.Timeline().Position = layout.Position{}
}

func (s *Shell) rememberComposerDraft(sessionID, draft string) {
	s.conversationUI.SaveDraft(sessionID, draft)
}

func (s *Shell) takeComposerDraft(sessionID string) string {
	return s.conversationUI.TakeDraft(sessionID)
}

func (s *Shell) forgetComposerDraft(sessionID string) {
	s.conversationUI.ForgetDraft(sessionID)
}
