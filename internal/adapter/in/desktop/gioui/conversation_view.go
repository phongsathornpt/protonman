//go:build desktop || desktop_gio

package gioui

import (
	"image/color"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/x/richtext"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	maxStreamingTextBytes        = 1 << 10
	maxConversationCacheEntries  = 512
	maxConversationCacheBytes    = 4 << 20
	maxCachedMarkdownItemBytes   = 256 << 10
	maxConversationExpansionKeys = 256
	largeMessagePreviewBytes     = 32 << 10
	largeMessagePageBytes        = 32 << 10
	largeMessagePreviewLines     = 48
	markdownSpanRetainedEstimate = 192
)

type conversationMarkdownCache struct {
	source string
	spans  []richtext.SpanStyle
	bytes  int
	plain  bool
}

type conversationCacheKey struct {
	sessionID string
	itemID    string
	index     int
	kind      desktopstate.TimelineKind
}

type conversationDisclosureButtons struct {
	show     widget.Clickable
	previous widget.Clickable
	next     widget.Clickable
	collapse widget.Clickable
}

func (s *shell) syncConversation(state desktopstate.State) {
	if state.ActiveSessionID == s.activeSessionID {
		s.syncPermissionButtons(state)
		return
	}
	s.activeSessionID = state.ActiveSessionID
	s.composer.SetText("")
	s.conversationList.Position = layout.Position{}
	s.inspectorList.Position = layout.Position{}
	s.inspectorOverride = false
	s.inspectorVisible = false
	s.runtimeEditorKey = ""
	clear(s.conversationCache)
	s.conversationCacheBytes = 0
	clear(s.conversationExpanded)
	clear(s.conversationPage)
	clear(s.conversationExpandButtons)
	clear(s.conversationThinkingExpanded)
	clear(s.conversationThinkingButtons)
	clear(s.permissionButtons)
	s.permissionButtonRevision = 0
	s.permissionButtonRevisionSet = false
}

func (s *shell) syncPermissionButtons(state desktopstate.State) {
	if s.syncRevision != 0 && s.permissionButtonRevisionSet && s.permissionButtonRevision == s.syncRevision {
		return
	}
	live := s.permissionButtonLive
	if live == nil {
		live = make(map[string]struct{}, len(state.PermissionInbox))
		s.permissionButtonLive = live
	}
	clear(live)
	for _, request := range state.PermissionInbox {
		live[request.RequestID] = struct{}{}
	}
	for requestID := range s.permissionButtons {
		if _, ok := live[requestID]; !ok {
			delete(s.permissionButtons, requestID)
		}
	}
	if s.syncRevision != 0 {
		s.permissionButtonRevision = s.syncRevision
		s.permissionButtonRevisionSet = true
	}
}

func (s *shell) layoutConversation(gtx layout.Context, session desktopstate.SessionState, history historyState) layout.Dimensions {
	gtx.Constraints.Min.X = 0
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(840))
	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(840))
		children := make([]layout.FlexChild, 0, 2)
		if history == historyStateLoading || session.HistoryTruncated {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutHistoryBanner(gtx, session.HistoryTruncated)
			}))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(session.Timeline) == 0 && len(session.Subagents) == 0 {
				return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(560))
					return layout.Inset{Top: 32, Bottom: 32, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainer, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: 24, Bottom: 24, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, "✦", textHeadlineMedium, font.Bold, s.theme.primary, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "Start a conversation", textHeadlineSmall, font.Bold, s.theme.onSurface, 1)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: 8, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "Send a prompt below. Protonman orchestrates deep reasoning, subagents, and durable memory to solve complex engineering tasks.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 4)
										})
									}),
								)
							})
						})
					})
				}))
			}
			runningActive := session.Status == desktopstate.TaskRunning
			itemCount := len(session.Timeline) + len(session.Subagents)
			if runningActive {
				itemCount++
			}
			return s.conversationList.Layout(gtx, itemCount, func(gtx layout.Context, index int) layout.Dimensions {
				if index < len(session.Timeline) {
					return s.layoutTimelineItem(gtx, session.ID, index, session.Timeline[index])
				}
				subagentIdx := index - len(session.Timeline)
				if subagentIdx < len(session.Subagents) {
					return s.layoutSubagentItem(gtx, session.Subagents[subagentIdx])
				}
				return s.layoutActiveThinkingIndicator(gtx, session)
			})
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}))
}

func (s *shell) layoutHistoryBanner(gtx layout.Context, truncated bool) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(40)
	return s.roundedSurface(gtx, shapeMedium, s.theme.secondaryContainer, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 10, Bottom: 10, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			message := "Loading session history…"
			if truncated {
				message = "Showing recent history; older entries were trimmed to keep the session responsive."
			}
			return s.layoutLabel(gtx, message, textLabelLarge, font.Medium, s.theme.onSecondaryContainer, 2)
		})
	})
}

type parsedAssistantMessage struct {
	hasThinking  bool
	thinkingDone bool
	thinkingText string
	responseText string
}

func parseAssistantThinking(text string) parsedAssistantMessage {
	const openTag = "<think>"
	const closeTag = "</think>"

	openIdx := strings.Index(text, openTag)
	if openIdx == -1 {
		return parsedAssistantMessage{
			hasThinking:  false,
			responseText: text,
		}
	}

	beforeThink := text[:openIdx]
	afterOpen := text[openIdx+len(openTag):]

	closeIdx := strings.Index(afterOpen, closeTag)
	if closeIdx == -1 {
		return parsedAssistantMessage{
			hasThinking:  true,
			thinkingDone: false,
			thinkingText: strings.TrimSpace(afterOpen),
			responseText: strings.TrimSpace(beforeThink),
		}
	}

	thinking := afterOpen[:closeIdx]
	afterClose := afterOpen[closeIdx+len(closeTag):]

	resp := strings.TrimSpace(beforeThink)
	trimmedAfter := strings.TrimSpace(afterClose)
	if resp != "" && trimmedAfter != "" {
		resp += "\n\n" + trimmedAfter
	} else if resp == "" {
		resp = trimmedAfter
	}

	return parsedAssistantMessage{
		hasThinking:  true,
		thinkingDone: true,
		thinkingText: strings.TrimSpace(thinking),
		responseText: resp,
	}
}

func (s *shell) layoutThinkingBlock(gtx layout.Context, key conversationCacheKey, parsed parsedAssistantMessage) layout.Dimensions {
	if s.conversationThinkingButtons == nil {
		s.conversationThinkingButtons = make(map[conversationCacheKey]*widget.Clickable)
	}
	btn := s.conversationThinkingButtons[key]
	if btn == nil {
		btn = new(widget.Clickable)
		s.conversationThinkingButtons[key] = btn
	}
	if btn.Clicked(gtx) {
		s.conversationThinkingExpanded[key] = !s.conversationThinkingExpanded[key]
	}

	expanded, hasExplicit := s.conversationThinkingExpanded[key]
	if !hasExplicit {
		expanded = !parsed.thinkingDone
	}

	statusLabel := "✦ Thought process"
	if !parsed.thinkingDone {
		statusLabel = "✦ Thinking in progress…"
	}

	toggleLabel := "▸ Show thoughts"
	if expanded {
		toggleLabel = "▾ Hide thoughts"
	}

	return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerLow, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, statusLabel, textLabelMedium, font.SemiBold, s.theme.tertiary, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutButton(gtx, btn, toggleLabel, true, func() {
									s.conversationThinkingExpanded[key] = !expanded
								})
							}),
						)
					}),
				}
				if expanded && strings.TrimSpace(parsed.thinkingText) != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, parsed.thinkingText, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 0)
								})
							})
						})
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func (s *shell) layoutActiveThinkingIndicator(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	label := "✦ Protonman is working…"
	if session.Runtime.Reasoning != "" && session.Runtime.Reasoning != "none" {
		label = "✦ Thinking & working (reasoning: " + session.Runtime.Reasoning + ")…"
	}

	return layout.Inset{Top: 6, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeFull, s.theme.surfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, s.theme.primary, 1)
					}),
				)
			})
		})
	})
}

func toolIconForTitle(title string) string {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "bash") || strings.Contains(lower, "sh") || strings.Contains(lower, "cmd"):
		return "⚡"
	case strings.Contains(lower, "edit") || strings.Contains(lower, "write") || strings.Contains(lower, "replace"):
		return "✎"
	case strings.Contains(lower, "read") || strings.Contains(lower, "cat"):
		return "📖"
	case strings.Contains(lower, "grep"):
		return "🔍"
	case strings.Contains(lower, "find") || strings.Contains(lower, "ls"):
		return "📁"
	case strings.Contains(lower, "git"):
		return "⎇"
	case strings.Contains(lower, "web") || strings.Contains(lower, "http"):
		return "🌐"
	case strings.Contains(lower, "todo"):
		return "📋"
	case strings.Contains(lower, "goal"):
		return "🎯"
	default:
		return "⚙"
	}
}

func (s *shell) layoutTimelineItem(gtx layout.Context, sessionID string, index int, item desktopstate.TimelineItem) layout.Dimensions {
	descriptionItem := item
	if item.Streaming {
		descriptionItem.Text = streamingText(item.Text)
	}

	if item.Kind == desktopstate.TimelineUser {
		return layout.Inset{Top: 6, Bottom: 6, Left: 48, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.roundedSurface(gtx, shapeLarge, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(conversationItemDescription(descriptionItem)).Add(gtx.Ops)
				return layout.Inset{Top: 12, Bottom: 12, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "You", textLabelMedium, font.SemiBold, s.theme.onPrimaryContainer, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, item.Text, textBodyMedium, font.Normal, s.theme.onPrimaryContainer, 0)
							})
						}),
					)
				})
			})
		})
	}

	if item.Kind == desktopstate.TimelineTool {
		return s.layoutToolItem(gtx, item, s.theme.onSurface)
	}

	if item.Kind == desktopstate.TimelineStatus {
		return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.roundedSurface(gtx, shapeMedium, s.theme.errorContainer, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(conversationItemDescription(descriptionItem)).Add(gtx.Ops)
				return layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, item.Text, textBodyMedium, font.Medium, s.theme.onErrorContainer, 0)
				})
			})
		})
	}

	// Assistant message
	key := makeConversationCacheKey(sessionID, index, item)
	parsed := parseAssistantThinking(item.Text)
	foreground := s.theme.onSurface

	return layout.Inset{Top: 6, Bottom: 6, Left: 6, Right: 32}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainer, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp(conversationItemDescription(descriptionItem)).Add(gtx.Ops)
			return layout.Inset{Top: 14, Bottom: 14, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Protonman", textLabelLarge, font.Bold, s.theme.primary, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "AI", textLabelSmall, font.SemiBold, s.theme.onPrimaryContainer, 1)
										})
									})
								})
							}),
						)
					}),
				}

				if parsed.hasThinking {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutThinkingBlock(gtx, key, parsed)
						})
					}))
				}

				if parsed.responseText != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if item.Streaming {
								return s.layoutLabel(gtx, streamingText(parsed.responseText), textBodyMedium, font.Normal, foreground, 6)
							}
							if len(parsed.responseText) > maxCachedMarkdownItemBytes {
								return s.layoutLargeMessage(gtx, key, parsed.responseText, foreground)
							}
							return s.layoutMarkdown(gtx, key, parsed.responseText, foreground)
						})
					}))
				} else if parsed.hasThinking && !parsed.thinkingDone {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Thinking and reasoning…", textLabelMedium, font.Normal, s.theme.onSurfaceVariant, 1)
						})
					}))
				}

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func (s *shell) layoutLargeMessage(gtx layout.Context, key conversationCacheKey, source string, foreground color.NRGBA) layout.Dimensions {
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
	pageCount := (len(source) + largeMessagePageBytes - 1) / largeMessagePageBytes
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
					return s.layoutButton(gtx, &buttons.collapse, "Show less", true, func() {
						s.conversationExpanded[key] = false
						s.conversationPage[key] = 0
					})
				}),
			)
		}),
	)
}

func (s *shell) largeMessageDisclosureButtons(key conversationCacheKey) *conversationDisclosureButtons {
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
		if len(s.conversationExpandButtons) >= maxConversationExpansionKeys {
			clear(s.conversationExpanded)
			clear(s.conversationPage)
			clear(s.conversationExpandButtons)
		}
		buttons = new(conversationDisclosureButtons)
		s.conversationExpandButtons[key] = buttons
	}
	return buttons
}

func largeMessageWindow(source string, page int) (string, int, int) {
	pageCount := (len(source) + largeMessagePageBytes - 1) / largeMessagePageBytes
	if page < 0 {
		page = 0
	} else if page >= pageCount {
		page = pageCount - 1
	}
	start := page * largeMessagePageBytes
	for start > 0 && start < len(source) && !utf8.RuneStart(source[start]) {
		start--
	}
	end := min((page+1)*largeMessagePageBytes, len(source))
	for end > start && end < len(source) && !utf8.RuneStart(source[end]) {
		end--
	}
	return strings.Clone(source[start:end]), start, end
}

func largeMessagePreview(source string) string {
	if len(source) <= largeMessagePreviewBytes {
		return source
	}
	end := largeMessagePreviewBytes
	for end > 0 && !utf8.RuneStart(source[end]) {
		end--
	}
	return source[:end] + "\n…"
}

func streamingText(source string) string {
	if len(source) <= maxStreamingTextBytes {
		return source
	}
	start := len(source) - maxStreamingTextBytes
	for start < len(source) && !utf8.RuneStart(source[start]) {
		start++
	}
	return "…\n" + source[start:]
}

func (s *shell) layoutToolItem(gtx layout.Context, item desktopstate.TimelineItem, foreground color.NRGBA) layout.Dimensions {
	status := strings.TrimSpace(item.Status)
	statusBg := s.theme.secondaryContainer
	statusFg := s.theme.onSecondaryContainer
	statusLabel := "TOOL"
	if status != "" {
		statusLabel = strings.ToUpper(status)
		switch strings.ToLower(status) {
		case "completed", "success", "ok":
			statusBg = s.theme.successContainer
			statusFg = s.theme.onSuccessContainer
		case "running", "in_progress":
			statusBg = s.theme.primaryContainer
			statusFg = s.theme.onPrimaryContainer
		case "failed", "error":
			statusBg = s.theme.errorContainer
			statusFg = s.theme.onErrorContainer
		}
	}

	icon := toolIconForTitle(item.Title)

	return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp(conversationItemDescription(item)).Add(gtx.Ops)
			return layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, icon+" "+item.Title, textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, statusBg, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, statusLabel, textLabelSmall, font.Bold, statusFg, 1)
									})
								})
							}),
						)
					}),
				}
				if strings.TrimSpace(item.Text) != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, item.Text, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 6)
								})
							})
						})
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func (s *shell) layoutSubagentItem(gtx layout.Context, subagent desktopstate.SubagentState) layout.Dimensions {
	profile := strings.ToLower(strings.TrimSpace(subagent.Profile))
	badgeText := "SUBAGENT"
	badgeBg := s.theme.secondaryContainer
	badgeFg := s.theme.onSecondaryContainer

	switch profile {
	case "strength", "str":
		badgeText = "⚔ STR · STRENGTH"
		badgeBg = s.theme.strengthContainer
		badgeFg = s.theme.onStrengthContainer
	case "agility", "agi":
		badgeText = "⚡ AGI · AGILITY"
		badgeBg = s.theme.agilityContainer
		badgeFg = s.theme.onAgilityContainer
	case "intelligence", "int":
		badgeText = "🧠 INT · INTELLIGENCE"
		badgeBg = s.theme.intelligenceContainer
		badgeFg = s.theme.onIntelligenceContainer
	default:
		if profile != "" {
			badgeText = "🤖 " + strings.ToUpper(profile)
		}
	}

	status := strings.ToUpper(strings.TrimSpace(subagent.Status))
	if status == "" {
		status = "PENDING"
	}
	statusBg := s.theme.surfaceContainerLow
	statusFg := s.theme.onSurfaceVariant
	switch strings.ToLower(status) {
	case "completed", "integrated":
		statusBg = s.theme.successContainer
		statusFg = s.theme.onSuccessContainer
	case "running", "farming", "roaming", "sticking":
		statusBg = s.theme.primaryContainer
		statusFg = s.theme.onPrimaryContainer
	case "failed", "b", "care":
		statusBg = s.theme.errorContainer
		statusFg = s.theme.onErrorContainer
	}

	return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp("Delegated agent " + subagent.Profile + ", " + subagent.Status + ", " + subagent.Task).Add(gtx.Ops)
			return layout.Inset{Top: 12, Bottom: 12, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, badgeBg, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, badgeText, textLabelMedium, font.Bold, badgeFg, 1)
									})
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, statusBg, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, status, textLabelSmall, font.SemiBold, statusFg, 1)
									})
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if strings.TrimSpace(subagent.Task) == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, subagent.Task, textBodyMedium, font.Medium, s.theme.onSurface, 3)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if strings.TrimSpace(subagent.Summary) == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, subagent.Summary, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 6)
								})
							})
						})
					}),
				)
			})
		})
	})
}

func (s *shell) layoutMarkdown(gtx layout.Context, key conversationCacheKey, source string, fallback color.NRGBA) layout.Dimensions {
	if strings.TrimSpace(source) == "" {
		s.dropMarkdownCache(key)
		return layout.Dimensions{}
	}
	if len(source) > maxCachedMarkdownItemBytes {
		s.dropMarkdownCache(key)
		return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
	}
	cached, ok := s.conversationCache[key]
	if !ok || cached.source != source {
		spans, err := s.conversationMarkdown.Render([]byte(source))
		if err != nil {
			s.dropMarkdownCache(key)
			cached = conversationMarkdownCache{source: source, plain: true}
			if !s.storeMarkdownCache(key, cached) {
				return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
			}
			cached = s.conversationCache[key]
		} else {
			for index := range spans {
				spans[index].Interactive = false
			}
			cached = conversationMarkdownCache{source: source, spans: spans}
			if !s.storeMarkdownCache(key, cached) {
				return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
			}
			cached = s.conversationCache[key]
		}
	}
	if cached.plain {
		return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
	}
	return richtext.Text(nil, s.theme.material.Shaper, cached.spans...).Layout(gtx)
}

func markdownCacheEntryBytes(key conversationCacheKey, cached conversationMarkdownCache) int {
	bytes := 128 + len(key.sessionID) + len(key.itemID) + len(cached.source)
	for _, span := range cached.spans {
		bytes += markdownSpanRetainedEstimate + len(span.Content)
	}
	return bytes
}

func (s *shell) storeMarkdownCache(key conversationCacheKey, cached conversationMarkdownCache) bool {
	cached.bytes = markdownCacheEntryBytes(key, cached)
	if cached.bytes > maxConversationCacheBytes {
		cached.spans = nil
		cached.plain = true
		cached.bytes = markdownCacheEntryBytes(key, cached)
		if cached.bytes > maxConversationCacheBytes {
			s.dropMarkdownCache(key)
			return false
		}
	}
	if previous, ok := s.conversationCache[key]; ok {
		s.conversationCacheBytes -= previous.bytes
		delete(s.conversationCache, key)
	}
	if len(s.conversationCache) >= maxConversationCacheEntries || s.conversationCacheBytes+cached.bytes > maxConversationCacheBytes {
		clear(s.conversationCache)
		s.conversationCacheBytes = 0
	}
	s.conversationCache[key] = cached
	s.conversationCacheBytes += cached.bytes
	return true
}

func (s *shell) dropMarkdownCache(key conversationCacheKey) {
	if cached, ok := s.conversationCache[key]; ok {
		delete(s.conversationCache, key)
		s.conversationCacheBytes -= cached.bytes
	}
}

func (s *shell) layoutPermissionPanel(gtx layout.Context, request desktopstate.PermissionRequest) layout.Dimensions {
	if s.permissionButtons[request.RequestID] == nil {
		s.permissionButtons[request.RequestID] = make(map[string]*widget.Clickable)
	}
	for _, option := range request.Options {
		if s.permissionButtons[request.RequestID][option.ID] == nil {
			s.permissionButtons[request.RequestID][option.ID] = new(widget.Clickable)
		}
	}
	return s.roundedSurface(gtx, shapeMedium, s.theme.warningContainer, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 16, Bottom: 16, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Permission required · "+request.Title, textTitleMedium, font.SemiBold, s.theme.onWarningContainer, 2)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, request.Detail, textBodyMedium, font.Normal, s.theme.onWarningContainer, 4)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, permissionOptionChildren(s, request)...)
				}),
			)
		})
	})
}

func permissionOptionChildren(s *shell, request desktopstate.PermissionRequest) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(request.Options))
	for _, option := range request.Options {
		option := option
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, s.permissionButtons[request.RequestID][option.ID], option.Name, true, func() {
					s.onResolvePermission(request.RequestID, option.ID)
				})
			})
		}))
	}
	return children
}

func (s *shell) layoutComposer(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	busy := sessionBusy(session.Status)
	connected := sessionConnection(snapshot, session.AgentID) == connectionConnected
	loading := snapshot.HistoryState == historyStateLoading
	canSend := connected && !busy && !loading
	s.composer.ReadOnly = !canSend
	if canSend {
		for {
			event, ok := s.composer.Update(gtx)
			if !ok {
				break
			}
			if submit, ok := event.(widget.SubmitEvent); ok {
				text := strings.TrimSpace(submit.Text)
				if text != "" {
					s.onSendPrompt(text)
					s.composer.SetText("")
				}
			}
		}
	}

	gtx.Constraints.Min.Y = gtx.Dp(118)
	return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainer, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 12, Bottom: 12, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutComposerContextChips(gtx, session)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutComposerEditor(gtx, canSend)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										helper := "Enter sends · Shift+Enter adds a line"
										if busy {
											helper = "The active turn can be cancelled without replaying it"
										} else if loading {
											helper = "Prompting resumes after session history finishes loading"
										} else if !connected {
											helper = "Reconnect to the ACP runtime before sending"
										}
										return s.layoutLabel(gtx, helper, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
									}),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Dp(96)
								return layout.Inset{Left: 10, Bottom: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									if busy {
										return s.layoutDangerButton(gtx, &s.stopButton, "Stop", connected, s.onCancelPrompt)
									}
									return s.layoutPrimaryButton(gtx, &s.sendButton, "Send ↵", canSend && strings.TrimSpace(s.composer.Text()) != "", func() {
										text := strings.TrimSpace(s.composer.Text())
										if text == "" {
											return
										}
										s.onSendPrompt(text)
										s.composer.SetText("")
									})
								})
							}),
						)
					})
				}),
			)
		})
	})
}

func (s *shell) layoutComposerContextChips(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	chips := make([]layout.FlexChild, 0, 3)

	if goal := strings.TrimSpace(session.Context.Goal); goal != "" {
		chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeSmall, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "🎯 "+compactInspectorText(goal, 30), textLabelSmall, font.Medium, s.theme.onPrimaryContainer, 1)
					})
				})
			})
		}))
	}

	if model := strings.TrimSpace(session.Runtime.Model); model != "" {
		chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeSmall, s.theme.secondaryContainer, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "⚡ "+compactInspectorText(model, 24), textLabelSmall, font.Normal, s.theme.onSecondaryContainer, 1)
					})
				})
			})
		}))
	}

	if reasoning := strings.TrimSpace(session.Runtime.Reasoning); reasoning != "" && reasoning != "none" {
		chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeSmall, s.theme.tertiaryContainer, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "✦ "+reasoning, textLabelSmall, font.Normal, s.theme.onTertiaryContainer, 1)
					})
				})
			})
		}))
	}

	if len(chips) == 0 {
		return layout.Dimensions{}
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, chips...)
}

func (s *shell) layoutComposerEditor(gtx layout.Context, enabled bool) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(64)
	gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(88))
	textColor := s.theme.onSurface
	if !enabled {
		textColor = s.theme.onSurfaceVariant
	}
	textMaterial := op.Record(gtx.Ops)
	paint.ColorOp{Color: textColor}.Add(gtx.Ops)
	textCall := textMaterial.Stop()
	selectionMaterial := op.Record(gtx.Ops)
	paint.ColorOp{Color: s.theme.primaryContainer}.Add(gtx.Ops)
	selectionCall := selectionMaterial.Stop()
	dims := s.roundedSurface(gtx, shapeSmall, s.theme.surface, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(64)
		gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(88))
		return layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.composer.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(font.Normal), textBodyMedium, textCall, selectionCall)
		})
	})
	if gtx.Focused(&s.composer) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	} else {
		widget.Border{Color: s.theme.outlineVariant, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func activePermission(state desktopstate.State, sessionID string) *desktopstate.PermissionRequest {
	for _, request := range state.PermissionInbox {
		if request.SessionID == sessionID {
			item := request
			item.Options = append([]desktopstate.PermissionOption(nil), request.Options...)
			return &item
		}
	}
	return nil
}

func makeConversationCacheKey(sessionID string, index int, item desktopstate.TimelineItem) conversationCacheKey {
	return conversationCacheKey{
		sessionID: sessionID,
		itemID:    item.ID,
		index:     index,
		kind:      item.Kind,
	}
}

func conversationItemDescription(item desktopstate.TimelineItem) string {
	var parts [4]string
	partCount := 0
	parts[partCount] = string(item.Kind)
	partCount++
	if item.Title != "" {
		parts[partCount] = item.Title
		partCount++
	}
	if item.Status != "" {
		parts[partCount] = item.Status
		partCount++
	}
	if item.Text != "" {
		parts[partCount] = item.Text
		partCount++
	}
	const maxDescriptionRunes = 512
	var description strings.Builder
	capacity := max(0, partCount-1) * 2
	for _, part := range parts[:partCount] {
		capacity += len(part)
	}
	description.Grow(min(capacity, maxDescriptionRunes*4))
	runeCount := 0
	prefixEnd := 0
	appendText := func(value string) bool {
		for index := 0; index < len(value); {
			if runeCount == maxDescriptionRunes {
				return false
			}
			_, width := utf8.DecodeRuneInString(value[index:])
			description.WriteString(value[index : index+width])
			index += width
			runeCount++
			if runeCount == maxDescriptionRunes-1 {
				prefixEnd = description.Len()
			}
		}
		return true
	}
	for index, part := range parts[:partCount] {
		if index > 0 && !appendText(", ") {
			return strings.TrimSpace(description.String()[:prefixEnd]) + "…"
		}
		if !appendText(part) {
			return strings.TrimSpace(description.String()[:prefixEnd]) + "…"
		}
	}
	return strings.TrimSpace(description.String())
}
