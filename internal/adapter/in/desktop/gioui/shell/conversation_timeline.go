//go:build desktop || desktop_gio

package shell

import (
	"fmt"
	"image/color"
	"io"
	"strconv"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Timeline layout: the ordered list of timeline items, per-item disclosure
// controls, and the jump-to-bottom affordance.
func (s *Shell) layoutConversation(gtx layout.Context, session desktopstate.SessionState, history controller.HistoryState) layout.Dimensions {
	return s.conversationUI.LayoutTranscript(gtx, conversationcomponent.TranscriptInput{
		Session: session, HistoryLoading: history == controller.HistoryStateLoading,
		OverlayOpen: func() bool {
			return s.runtimeComponent.Widgets().ModelPopoverVisible || s.runtimeComponent.Widgets().ReasoningPopoverVisible || s.runtimeComponent.Widgets().PermissionModePopoverVisible || (s.mentionActive && len(s.mentionItems) > 0)
		},
		MaxTextWidth: maxConversationTextWidth,
		Chrome: conversationcomponent.TranscriptChrome{
			SecondaryContainer: s.theme.Colors.SecondaryContainer, OnSecondaryContainer: s.theme.Colors.OnSecondaryContainer,
			Label: s.layoutLabel, RoundedSurface: s.roundedSurface,
		},
		Empty: func(gtx layout.Context) layout.Dimensions { return s.layoutEmptyState(gtx, session) },
		TimelineItem: func(gtx layout.Context, index int, item desktopstate.TimelineItem) layout.Dimensions {
			return s.layoutTimelineItem(gtx, session.ID, index, item)
		},
		SubagentItem:   s.layoutSubagentItem,
		ActiveThinking: s.layoutActiveThinkingIndicator,
		JumpToBottom:   s.layoutJumpToBottomButton,
	})
}

func conversationButtonKey(sessionID, role string, index int, item desktopstate.TimelineItem) string {
	if item.ID != "" {
		return sessionID + ":" + role + ":" + item.ID
	}
	return sessionID + ":" + role + ":#" + fmt.Sprint(index)
}

func (s *Shell) layoutTimelineItem(gtx layout.Context, sessionID string, index int, item desktopstate.TimelineItem) layout.Dimensions {
	descriptionItem := item
	if item.Streaming {
		descriptionItem.Text = streamingText(item.Text)
	}
	itemKey := makeConversationCacheKey(sessionID, index, item)

	if item.Kind == desktopstate.TimelineUser {
		msgKey := conversationButtonKey(sessionID, "user", index, item)
		return uikit.Inset{Top: 8, Bottom: 8, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp(s.conversationDescription(itemKey, descriptionItem)).Add(gtx.Ops)
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					maxUserWidth := min(gtx.Dp(680), int(float32(gtx.Constraints.Max.X)*0.75))
					if maxUserWidth < gtx.Dp(260) {
						maxUserWidth = min(gtx.Constraints.Max.X, gtx.Dp(680))
					}
					gtx.Constraints.Max.X = maxUserWidth
					return s.roundedBorderSurface(gtx, shapeLarge, s.theme.Colors.SurfaceContainerHigh, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 8, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "You", textLabelSmall, font.SemiBold, s.theme.Colors.Primary, 1)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Left: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutUserRetryButton(gtx, msgKey, item.Text)
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutMessageCopyButton(gtx, msgKey, item.Text)
											})
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, item.Text, textBodyMedium, font.Normal, s.theme.Colors.OnSurface, 0)
									})
								}),
							)
						})
					})
				}),
			)
		})
	}

	if item.Kind == desktopstate.TimelineTool {
		return s.layoutToolItem(gtx, sessionID, index, item, s.theme.Colors.OnSurface)
	}

	if item.Kind == desktopstate.TimelinePermission {
		return s.layoutPermissionAuditItem(gtx, item)
	}

	if item.Kind == desktopstate.TimelineStatus {
		return uikit.Inset{Top: 6, Bottom: 6, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.roundedSurface(gtx, shapeMedium, s.theme.Colors.ErrorContainer, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(conversationItemDescription(descriptionItem)).Add(gtx.Ops)
				return uikit.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, item.Text, textBodyMedium, font.Medium, s.theme.Colors.OnErrorContainer, 0)
				})
			})
		})
	}

	// Assistant message
	key := makeConversationCacheKey(sessionID, index, item)
	parsed := s.parsedThinking(key, item.Text)
	foreground := s.theme.Colors.OnSurface

	return uikit.Inset{Top: 8, Bottom: 12, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.DescriptionOp(s.conversationDescription(key, descriptionItem)).Add(gtx.Ops)
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				msgKey := conversationButtonKey(sessionID, "asst", index, item)
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.LayoutActionIcon(gtx, uikit.KindBrandLogo, 16, s.theme.Colors.Primary)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Protonman", textLabelLarge, font.Bold, s.theme.Colors.Primary, 1)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "AI", textLabelSmall, font.SemiBold, s.theme.Colors.OnPrimaryContainer, 1)
								})
							})
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Spacer{}.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						copyContent := parsed.responseText
						if copyContent == "" {
							copyContent = item.Text
						}
						if strings.TrimSpace(copyContent) == "" {
							return layout.Dimensions{}
						}
						return s.layoutMessageCopyButton(gtx, msgKey, copyContent)
					}),
				)
			}),
		}

		if parsed.hasThinking {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutThinkingBlock(gtx, key, parsed)
				})
			}))
		}

		if parsed.responseText != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if item.Streaming {
						return s.layoutLabel(gtx, streamingText(parsed.responseText), textBodyMedium, font.Normal, foreground, 6)
					}
					if len(parsed.responseText) > maxMarkdownRenderBytes {
						return s.layoutLargeMessage(gtx, key, parsed.responseText, foreground)
					}
					return s.layoutRichResponse(gtx, key, s.responseBlocks(key, parsed.responseText), foreground)
				})
			}))
		} else if parsed.hasThinking && !parsed.thinkingDone {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Thinking and reasoning…", textLabelMedium, font.Normal, s.theme.Colors.OnSurfaceVariant, 1)
				})
			}))
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (s *Shell) layoutLargeMessage(gtx layout.Context, key conversationCacheKey, source string, foreground color.NRGBA) layout.Dimensions {
	buttons := s.largeMessageDisclosureButtons(key)
	expanded := s.conversationExpanded[key]
	if !expanded {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, largeMessagePreview(source), textBodyMedium, font.Normal, foreground, largeMessagePreviewLines)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &buttons.show, "Show full message", true, func() {
					s.conversationExpanded[key] = true
					s.conversationPage[key] = 0
				})
			}),
		)
	}

	page := s.conversationPage[key]
	pageCount := (len(source) + messagePageBytes - 1) / messagePageBytes
	if page < 0 {
		page = 0
	} else if page >= pageCount {
		page = pageCount - 1
	}
	s.conversationPage[key] = page
	text, _, end := largeMessageWindow(source, page)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, text, textBodyMedium, font.Normal, foreground, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "Part " + strconv.Itoa(page+1) + " of " + strconv.Itoa(pageCount)
			return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, foreground, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutButton(gtx, &buttons.previous, "Previous part", page > 0, func() {
						if s.conversationPage[key] > 0 {
							s.conversationPage[key]--
						}
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutButton(gtx, &buttons.next, "Next part", end < len(source), func() {
						s.conversationPage[key]++
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutButton(gtx, &buttons.collapse, "Show less", true, func() {
							s.conversationExpanded[key] = false
							s.conversationPage[key] = 0
						})
					})
				}),
			)
		}),
	)
}

func (s *Shell) largeMessageDisclosureButtons(key conversationCacheKey) *conversationDisclosureButtons {
	if s.conversationExpanded == nil {
		s.conversationExpanded = make(map[conversationCacheKey]bool)
	}
	if s.conversationPage == nil {
		s.conversationPage = make(map[conversationCacheKey]int)
	}
	if s.conversationExpandButtons == nil {
		s.conversationExpandButtons = make(map[conversationCacheKey]*conversationDisclosureButtons)
	}
	buttons := s.conversationExpandButtons[key]
	if buttons == nil {
		s.admitExpansionKey(key)
		buttons = new(conversationDisclosureButtons)
		s.conversationExpandButtons[key] = buttons
	}
	return buttons
}

func (s *Shell) admitExpansionKey(key conversationCacheKey) {
	if oldest, _, evicted := s.expansionOrder.Put(key, struct{}{}); evicted {
		delete(s.conversationExpandButtons, oldest)
		delete(s.toolOutputPages, oldest)
		delete(s.conversationExpanded, oldest)
		delete(s.conversationPage, oldest)
	}
}

func largeMessageWindow(source string, page int) (string, int, int) {
	return conversationcomponent.LargeMessageWindow(source, page)
}

func largeMessagePreview(source string) string {
	return conversationcomponent.LargeMessagePreview(source)
}

func (s *Shell) layoutJumpToBottomButton(gtx layout.Context) layout.Dimensions {
	return s.conversationUI.LayoutJumpToBottom(gtx, conversationcomponent.JumpChrome{
		SurfaceContainerHighest: s.theme.Colors.SurfaceContainerHighest,
		PrimaryContainer:        s.theme.Colors.PrimaryContainer, OutlineVariant: s.theme.Colors.OutlineVariant,
		Primary: s.theme.Colors.Primary, OnPrimaryContainer: s.theme.Colors.OnPrimaryContainer,
		Icon: func(gtx layout.Context, tint color.NRGBA) layout.Dimensions {
			return uikit.LayoutActionIcon(gtx, uikit.KindChevronDown, 14, tint)
		},
		Label: s.layoutLabel, RoundedSurface: s.roundedSurface, BorderSurface: s.roundedBorderSurface,
	}, func() {
		s.closePopovers()
		gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
		s.conversationUI.Timeline().ScrollToEnd = true
		s.conversationUI.Timeline().Position = layout.Position{}
		gtx.Execute(op.InvalidateCmd{})
	})
}

func (s *Shell) layoutMessageCopyButton(gtx layout.Context, keyStr, text string) layout.Dimensions {
	if s.messageCopyButtons == nil {
		s.messageCopyButtons = make(map[string]*widget.Clickable)
	}
	btn, ok := s.messageCopyButtons[keyStr]
	if !ok {
		btn = new(widget.Clickable)
		s.messageCopyButtons[keyStr] = btn
	}

	if btn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(text)),
		})
		if s.messageCopiedAt == nil {
			s.messageCopiedAt = make(map[string]time.Time)
		}
		s.messageCopiedAt[keyStr] = time.Now()
	}

	copied := false
	if s.messageCopiedAt != nil {
		if t, ok := s.messageCopiedAt[keyStr]; ok && time.Since(t) < 2*time.Second {
			copied = true
		}
	}

	btnText := "Copy"
	btnColor := s.theme.Colors.OnSurfaceVariant
	if copied {
		btnText = "✓ Copied"
		btnColor = s.theme.Colors.OnSuccessContainer
	} else if btn.Hovered() {
		btnColor = s.theme.Colors.Primary
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if copied {
						return uikit.LayoutActionIcon(gtx, uikit.KindCheck, 12, btnColor)
					}
					return uikit.LayoutActionIcon(gtx, uikit.KindCopy, 12, btnColor)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, btnText, textLabelSmall, font.Medium, btnColor, 1)
					})
				}),
			)
		})
	})
}

func (s *Shell) layoutUserRetryButton(gtx layout.Context, keyStr, text string) layout.Dimensions {
	if s.userRetryButtons == nil {
		s.userRetryButtons = make(map[string]*widget.Clickable)
	}
	btn, ok := s.userRetryButtons[keyStr]
	if !ok {
		btn = new(widget.Clickable)
		s.userRetryButtons[keyStr] = btn
	}

	if btn.Clicked(gtx) {
		s.setComposerText(text)
		s.conversationUI.Editor().SetCaret(len(text), len(text))
		s.composerError = ""
		gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
	}

	btnColor := s.theme.Colors.OnSurfaceVariant
	if btn.Hovered() {
		btnColor = s.theme.Colors.Primary
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.LayoutActionIcon(gtx, uikit.KindCompose, 12, btnColor)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "Retry", textLabelSmall, font.Medium, btnColor, 1)
					})
				}),
			)
		})
	})
}
