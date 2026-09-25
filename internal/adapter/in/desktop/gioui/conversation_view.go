//go:build desktop || desktop_gio

package gioui

import (
	"image/color"
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

const maxStreamingTextBytes = 1 << 10

type conversationMarkdownCache struct {
	source string
	spans  []richtext.SpanStyle
}

type conversationCacheKey struct {
	sessionID string
	itemID    string
	index     int
	kind      desktopstate.TimelineKind
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
		if history == historyStateLoading {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutHistoryBanner(gtx)
			}))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(session.Timeline) == 0 && len(session.Subagents) == 0 {
				return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(520))
					return layout.Inset{Top: 24, Bottom: 24, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Start the conversation", textTitleMedium, font.SemiBold, s.theme.onSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Send a prompt below. Tool calls and delegated-agent activity appear in this session timeline.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 3)
								})
							}),
						)
					})
				}))
			}
			return s.conversationList.Layout(gtx, len(session.Timeline)+len(session.Subagents), func(gtx layout.Context, index int) layout.Dimensions {
				if index < len(session.Timeline) {
					return s.layoutTimelineItem(gtx, session.ID, index, session.Timeline[index])
				}
				return s.layoutSubagentItem(gtx, session.Subagents[index-len(session.Timeline)])
			})
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}))
}

func (s *shell) layoutHistoryBanner(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(40)
	return s.roundedSurface(gtx, 0, s.theme.secondaryContainer, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 10, Bottom: 10, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "Loading session history…", textLabelLarge, font.Medium, s.theme.onSecondaryContainer, 1)
		})
	})
}

func (s *shell) layoutTimelineItem(gtx layout.Context, sessionID string, index int, item desktopstate.TimelineItem) layout.Dimensions {
	background := s.theme.surfaceContainer
	foreground := s.theme.onSurface
	if item.Kind == desktopstate.TimelineUser {
		background = s.theme.primaryContainer
		foreground = s.theme.onPrimaryContainer
	} else if item.Kind == desktopstate.TimelineStatus {
		background = s.theme.errorContainer
		foreground = s.theme.onErrorContainer
	}
	return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, 16, background, func(gtx layout.Context) layout.Dimensions {
			descriptionItem := item
			if item.Streaming {
				descriptionItem.Text = streamingText(item.Text)
			}
			semantic.DescriptionOp(conversationItemDescription(descriptionItem)).Add(gtx.Ops)
			return layout.Inset{Top: 14, Bottom: 14, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if item.Kind == desktopstate.TimelineTool {
					return s.layoutToolItem(gtx, item, foreground)
				}
				if item.Kind == desktopstate.TimelineStatus {
					return s.layoutLabel(gtx, item.Text, textBodyMedium, font.Medium, foreground, 0)
				}
				if item.Streaming {
					return s.layoutLabel(gtx, descriptionItem.Text, textBodyMedium, font.Normal, foreground, 6)
				}
				key := makeConversationCacheKey(sessionID, index, item)
				return s.layoutMarkdown(gtx, key, item.Text, foreground)
			})
		})
	})
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
	children := []layout.FlexChild{
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, item.Title, textBodyLarge, font.SemiBold, foreground, 2)
		}),
	}
	if status := strings.TrimSpace(item.Status); status != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, strings.ToUpper(status), textLabelMedium, font.SemiBold, foreground, 1)
		}))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if strings.TrimSpace(item.Text) == "" {
			return layout.Dimensions{}
		}
		return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, item.Text, textBodyMedium, font.Normal, foreground, 6)
		})
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutSubagentItem(gtx layout.Context, subagent desktopstate.SubagentState) layout.Dimensions {
	return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, 16, s.theme.surfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp("Delegated agent " + subagent.Profile + ", " + subagent.Status + ", " + subagent.Task).Add(gtx.Ops)
			return layout.Inset{Top: 14, Bottom: 14, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				profile := strings.ToUpper(strings.TrimSpace(subagent.Profile))
				if profile == "" {
					profile = "AGENT"
				}
				status := strings.ToUpper(strings.TrimSpace(subagent.Status))
				if status == "" {
					status = "PENDING"
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, profile, textLabelLarge, font.Bold, s.theme.primary, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, status, textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if strings.TrimSpace(subagent.Task) == "" {
							return layout.Dimensions{}
						}
						return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, subagent.Task, textBodyMedium, font.Medium, s.theme.onSurface, 3)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if strings.TrimSpace(subagent.Summary) == "" {
							return layout.Dimensions{}
						}
						return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, subagent.Summary, textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 6)
						})
					}),
				)
			})
		})
	})
}

func (s *shell) layoutMarkdown(gtx layout.Context, key conversationCacheKey, source string, fallback color.NRGBA) layout.Dimensions {
	if strings.TrimSpace(source) == "" {
		return layout.Dimensions{}
	}
	cached, ok := s.conversationCache[key]
	if !ok || cached.source != source {
		spans, err := s.conversationMarkdown.Render([]byte(source))
		if err != nil {
			return s.layoutLabel(gtx, source, textBodyMedium, font.Normal, fallback, 0)
		}
		for index := range spans {
			spans[index].Interactive = false
		}
		cached = conversationMarkdownCache{source: source, spans: spans}
		if len(s.conversationCache) >= 512 {
			clear(s.conversationCache)
		}
		s.conversationCache[key] = cached
	}
	return richtext.Text(nil, s.theme.material.Shaper, cached.spans...).Layout(gtx)
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
	return s.roundedSurface(gtx, 0, s.theme.errorContainer, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 16, Bottom: 16, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Permission required · "+request.Title, textTitleMedium, font.SemiBold, s.theme.onErrorContainer, 2)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, request.Detail, textBodyMedium, font.Normal, s.theme.onErrorContainer, 4)
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
	return s.roundedSurface(gtx, 0, s.theme.surfaceContainer, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 14, Bottom: 14, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "MESSAGE PROTONMAN", textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
							return s.layoutLabel(gtx, helper, textLabelMedium, font.Normal, s.theme.onSurfaceVariant, 1)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(88)
					if busy {
						return s.layoutButton(gtx, &s.stopButton, "Stop", connected, s.onCancelPrompt)
					}
					return s.layoutButton(gtx, &s.sendButton, "Send", canSend && strings.TrimSpace(s.composer.Text()) != "", func() {
						text := strings.TrimSpace(s.composer.Text())
						if text == "" {
							return
						}
						s.onSendPrompt(text)
						s.composer.SetText("")
					})
				}),
			)
		})
	})
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
	dims := s.roundedSurface(gtx, 12, s.theme.surface, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(64)
		gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(88))
		return layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.composer.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(font.Normal), textBodyMedium, textCall, selectionCall)
		})
	})
	if gtx.Focused(&s.composer) {
		widget.Border{Color: s.theme.primary, CornerRadius: 12, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
	parts := []string{string(item.Kind)}
	if item.Title != "" {
		parts = append(parts, item.Title)
	}
	if item.Status != "" {
		parts = append(parts, item.Status)
	}
	if item.Text != "" {
		parts = append(parts, item.Text)
	}
	return compactInspectorText(strings.Join(parts, ", "), 512)
}
