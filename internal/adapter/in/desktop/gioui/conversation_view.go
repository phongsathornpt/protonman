//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/richtext"

	"github.com/phongsathornpt/protonman/internal/base/diffutil"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	maxComposerDrafts            = 32
	maxStreamingTextBytes        = 1 << 10
	maxConversationCacheEntries  = 512
	maxConversationCacheBytes    = 4 << 20
	maxConversationExpansionKeys = 256
	// maxMarkdownRenderBytes bounds what gets shaped through the markdown
	// richtext path. Text shaping cost scales with total message length even
	// though only a viewport is visible, so anything larger renders through
	// the paged plain-text path instead; profiling showed a 100 KiB message
	// shaping every frame dominated mixed-prose frame time.
	maxMarkdownRenderBytes        = 16 << 10
	maxResponseSplitCacheEntries  = 128
	maxResponseSplitCacheBytes    = 2 << 20
	maxResponseSplitSourceBytes   = 256 << 10
	responseSplitRetainedEstimate = 96
	markdownSpanRetainedEstimate  = 192
	maxToolDiffCacheEntries       = 512
	maxThinkingParseCacheEntries  = 512
	// Caps the per-item accessibility description memo; conversation frames
	// rebuild visible descriptions every redraw, which dominated frame
	// allocations in tool-heavy sessions.
	maxConversationDescriptionCacheEntries = 512
	// Paged large-message geometry. Previews and pages stay small so a fully
	// expanded message shapes at most one bounded page per frame.
	maxMessagePreviewBytes   = 8 << 10
	messagePageBytes         = 8 << 10
	largeMessagePreviewLines = 48
	// Tool outputs above this threshold render through the paged large-message
	// pattern instead of laying out the full text every frame. Expanded tool
	// bodies (terminal dumps in particular) previously laid out unbounded text
	// on every scroll frame.
	maxToolOutputUnpagedBytes = 8 << 10
)

// descriptionCacheEntry memoizes conversationItemDescription for one timeline
// item. Descriptions are rebuilt for every visible item on each frame; the
// full source item is retained for equality so streaming text invalidates
// naturally.
type descriptionCacheEntry struct {
	source      desktopstate.TimelineItem
	description string
}

// conversationDescription returns the cached accessibility description for a
// timeline item, rebuilding it only when the item changes. Keys use the same
// stable timeline identity as the other conversation caches; kind separation
// keeps user/assistant/tool items with matching IDs from colliding.
func (s *shell) conversationDescription(key conversationCacheKey, item desktopstate.TimelineItem) string {
	if cached, ok := s.conversationDescriptions.get(key); ok && cached.source == item {
		return cached.description
	}
	description := conversationItemDescription(item)
	s.conversationDescriptions.put(key, descriptionCacheEntry{source: item, description: description})
	return description
}

// toolDiffCacheEntry memoizes the per-frame classification of one tool item's
// output text (diff detection, stats, filename, preview).
type toolDiffCacheEntry struct {
	source    string
	isDiff    bool
	filename  string
	additions int
	deletions int
	preview   []string
	omitted   int
}

// thinkingParseCacheEntry memoizes parseAssistantThinking for one timeline item.
type thinkingParseCacheEntry struct {
	source string
	parsed parsedAssistantMessage
}

type conversationMarkdownCache struct {
	source string
	spans  []richtext.SpanStyle
	bytes  int
	plain  bool
}

type conversationCodeCache struct {
	source string
	lang   string
	spans  []richtext.SpanStyle
	bytes  int
}

type conversationResponseCache struct {
	source string
	blocks []markdownBlock
}

type conversationCacheKey struct {
	sessionID string
	itemID    string
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
	if s.activeSessionID != "" {
		if draft := s.composer.Text(); strings.TrimSpace(draft) != "" {
			s.rememberComposerDraft(s.activeSessionID, draft)
		} else {
			s.forgetComposerDraft(s.activeSessionID)
		}
	}
	s.activeSessionID = state.ActiveSessionID
	if state.ActiveSessionID != "" {
		rows, _ := buildSidebarRows(state, nil)
		for index, row := range rows {
			if row.Kind == sidebarSessionRow && row.SessionID == state.ActiveSessionID {
				s.sidebarList.Position = layout.Position{First: max(0, index-1)}
				break
			}
		}
	}
	s.setComposerText(s.takeComposerDraft(state.ActiveSessionID))
	s.closePopovers()
	s.tailFollowBeforeOverlay = false
	s.conversationList.Position = layout.Position{}
	s.inspectorList.Position = layout.Position{}
	s.inspectorOverride = false
	s.inspectorVisible = false
	s.runtimeEditorKey = ""
	clear(s.conversationResponseCache)
	s.conversationResponseBytes = 0
	s.conversationResponseOrder = s.conversationResponseOrder[:0]
	clear(s.conversationExpanded)
	clear(s.conversationPage)
	clear(s.conversationExpandButtons)
	clear(s.toolOutputPages)
	s.expansionOrder.reset()
	clear(s.conversationThinkingExpanded)
	clear(s.conversationThinkingButtons)
	s.conversationThinkingCache.reset()
	s.toolDiffCache.reset()
	clear(s.permissionButtons)
	clear(s.toolExpanded)
	clear(s.toolExpandButtons)
	clear(s.messageCopyButtons)
	clear(s.messageCopiedAt)
	clear(s.codeCopyButtons)
	clear(s.codeCopiedAt)
	clear(s.userRetryButtons)
	clear(s.mentionButtons)
	clear(s.agentModelButtons)
	clear(s.modelPresetButtons)
	clear(s.popoverReasoningButtons)
	clear(s.popoverPermissionModeButtons)
	clear(s.questionStates)
	s.permissionButtonRevision = 0
	s.permissionButtonRevisionSet = false
	s.mentionStateDirty = true
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
			delete(s.permissionRawToggles, requestID)
			delete(s.permissionRawClickable, requestID)
			delete(s.permissionCopyButtons, requestID)
		}
	}
	if s.syncRevision != 0 {
		s.permissionButtonRevision = s.syncRevision
		s.permissionButtonRevisionSet = true
	}
}

func (s *shell) layoutConversation(gtx layout.Context, session desktopstate.SessionState, history historyState) layout.Dimensions {
	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = 0
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(960))
		children := make([]layout.FlexChild, 0, 2)
		if history == historyStateLoading || session.HistoryTruncated {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 8, Bottom: 8, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutHistoryBanner(gtx, session.HistoryTruncated)
				})
			}))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(session.Timeline) == 0 && len(session.Subagents) == 0 {
				return s.layoutEmptyState(gtx, session)
			}
			runningActive := session.Status == desktopstate.TaskRunning
			itemCount := len(session.Timeline) + len(session.Subagents)
			if runningActive {
				itemCount++
			}
			// Extra clearance item at the bottom of the conversation stream
			itemCount++

			return layout.Stack{Alignment: layout.S}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					dims := s.conversationList.Layout(gtx, itemCount, func(gtx layout.Context, index int) layout.Dimensions {
						if index < len(session.Timeline) {
							return s.layoutTimelineItem(gtx, session.ID, index, session.Timeline[index])
						}
						subagentIdx := index - len(session.Timeline)
						if subagentIdx < len(session.Subagents) {
							return s.layoutSubagentItem(gtx, session.Subagents[subagentIdx])
						}
						if runningActive && index == len(session.Timeline)+len(session.Subagents) {
							return s.layoutActiveThinkingIndicator(gtx, session)
						}
						return layout.Spacer{Height: 48}.Layout(gtx)
					})
					composerOverlayOpen := s.modelPopoverVisible || s.reasoningPopoverVisible || s.permissionModePopoverVisible || (s.mentionActive && len(s.mentionItems) > 0)
					if s.conversationList.Position.BeforeEnd {
						s.conversationList.ScrollToEnd = false
					} else if !composerOverlayOpen {
						s.conversationList.ScrollToEnd = true
					} else {
						s.conversationList.ScrollToEnd = false
					}
					return dims
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if !s.conversationList.Position.BeforeEnd || len(session.Timeline) <= 2 {
						return layout.Dimensions{}
					}
					return desktopInset{Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutJumpToBottomButton(gtx)
					})
				}),
			)
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}))
}

func (s *shell) layoutHistoryBanner(gtx layout.Context, truncated bool) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(40)
	return s.roundedSurface(gtx, shapeMedium, s.theme.secondaryContainer, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 10, Bottom: 10, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			message := "Loading session history…"
			if truncated {
				message = "Showing recent history; older entries were trimmed to keep the session responsive."
			}
			return s.layoutLabel(gtx, message, textLabelLarge, font.Medium, s.theme.onSecondaryContainer, 2)
		})
	})
}

type starterPrompt struct {
	icon        iconKind
	title       string
	description string
	prompt      string
}

var starterPrompts = [4]starterPrompt{
	{
		icon:        iconSearch,
		title:       "Explain codebase",
		description: "Understand architecture & workflows",
		prompt:      "Explain the architecture, structure, and main workflows of this project.",
	},
	{
		icon:        iconFolder,
		title:       "Find & fix bugs",
		description: "Inspect changes & resolve edge cases",
		prompt:      "Inspect recent changes, find any bugs or edge cases, and propose fixes.",
	},
	{
		icon:        iconTerminal,
		title:       "Run tests & verify",
		description: "Execute test suite & analyze results",
		prompt:      "Run standard repository verification tests and report any failures.",
	},
	{
		icon:        iconCompose,
		title:       "Refactor & optimize",
		description: "Improve code clarity & performance",
		prompt:      "Help me refactor and optimize code for clarity, performance, and maintainability.",
	},
}

func (s *shell) layoutEmptyState(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
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
		return desktopInset{Top: 24, Bottom: 16, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Protonman", textHeadlineSmall, font.Bold, s.theme.onSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 4, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Autonomous coding agent for "+wsName, textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 1)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "Agent: "+agentName, textLabelSmall, font.SemiBold, s.theme.onPrimaryContainer, 1)
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerHighest, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, "Model: "+modelName, textLabelSmall, font.Medium, s.theme.onSurface, 1)
											})
										})
									})
								}),
							)
						}),
					)
				}),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 20, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						twoCol := gtx.Constraints.Max.X >= gtx.Dp(460)
						if twoCol {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											return s.layoutStarterCard(gtx, 0, starterPrompts[0])
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return layout.Spacer{Width: 10}.Layout(gtx)
										}),
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											return s.layoutStarterCard(gtx, 1, starterPrompts[1])
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												return s.layoutStarterCard(gtx, 2, starterPrompts[2])
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Spacer{Width: 10}.Layout(gtx)
											}),
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												return s.layoutStarterCard(gtx, 3, starterPrompts[3])
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
								return desktopInset{Top: topInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutStarterCard(gtx, idx, starterPrompts[idx])
								})
							}))
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
					})
				}),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.roundedSurface(gtx, shapeFull, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "@ mention files  ·  / commands  ·  Alt+M select model  ·  Cmd+N new chat", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
						})
					})
				}),
			)
		})
	}))
}

func (s *shell) layoutStarterCard(gtx layout.Context, index int, p starterPrompt) layout.Dimensions {
	btn := &s.starterPromptButtons[index]
	if btn.Clicked(gtx) {
		s.setComposerText(p.prompt)
		s.composer.SetCaret(len(p.prompt), len(p.prompt))
		s.composerError = ""
		s.conversationList.ScrollToEnd = true
		s.conversationList.Position = layout.Position{}
		gtx.Execute(key.FocusCmd{Tag: &s.composer})
	}

	bg := s.theme.surfaceContainerHigh
	borderColor := s.theme.outlineVariant
	if btn.Hovered() {
		bg = s.theme.surfaceContainerHighest
		borderColor = s.theme.primary
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, bg, borderColor, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutActionIcon(gtx, p.icon, 20, s.theme.primary)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, p.title, textLabelLarge, font.SemiBold, s.theme.onSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, p.description, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 2)
								})
							}),
						)
					}),
				)
			})
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

// parsedThinking memoizes parseAssistantThinking per timeline item. Assistant
// messages are re-parsed on every frame; caching by identity and source keeps a
// long, unchanged message from being re-scanned while the view redraws.
func (s *shell) parsedThinking(key conversationCacheKey, text string) parsedAssistantMessage {
	if cached, ok := s.conversationThinkingCache.get(key); ok && cached.source == text {
		return cached.parsed
	}
	parsed := parseAssistantThinking(text)
	s.conversationThinkingCache.put(key, thinkingParseCacheEntry{source: text, parsed: parsed})
	return parsed
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

	// An in-progress thinking block auto-expands, so it would shape its full
	// reasoning text on every frame; cap the live view to the streaming tail.
	// Once thinking completes the block starts collapsed, so the full text
	// only shapes when the user explicitly expands it.
	thinkingText := parsed.thinkingText
	if !parsed.thinkingDone {
		thinkingText = streamingText(parsed.thinkingText)
	}

	statusLabel := "Thought process"
	if !parsed.thinkingDone {
		statusLabel = "Thinking in progress…"
	} else if count := strings.Count(parsed.thinkingText, "\n") + 1; count > 1 {
		statusLabel = fmt.Sprintf("Thought process (%d lines)", count)
	}

	chevron := iconChevronRight
	if expanded {
		chevron = iconChevronDown
	}

	headerBg := s.theme.surfaceContainerLow
	headerBorder := s.theme.outlineVariant
	if btn.Hovered() {
		headerBg = s.theme.surfaceContainerHigh
		headerBorder = s.theme.tertiary
	}

	return desktopInset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedBorderSurface(gtx, shapeMedium, headerBg, headerBorder, 1, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, iconStar, 14, s.theme.tertiary)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, statusLabel, textLabelMedium, font.Medium, s.theme.tertiary, 1)
									})
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{}.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, chevron, 12, s.theme.onSurfaceVariant)
								}),
							)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !expanded || strings.TrimSpace(parsed.thinkingText) == "" {
					return layout.Dimensions{}
				}
				return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, thinkingText, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 0)
						})
					})
				})
			}),
		)
	})
}

func (s *shell) layoutActiveThinkingIndicator(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	label := "Protonman is working…"
	if session.Runtime.Reasoning != "" && session.Runtime.Reasoning != "none" {
		label = "Thinking and working (reasoning: " + session.Runtime.Reasoning + ")…"
	}

	return desktopInset{Top: 6, Bottom: 10, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeFull, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutActionIcon(gtx, iconStar, 14, s.theme.onPrimaryContainer)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, s.theme.onPrimaryContainer, 1)
						})
					}),
				)
			})
		})
	})
}

func (s *shell) layoutJumpToBottomButton(gtx layout.Context) layout.Dimensions {
	if s.jumpToBottomButton.Clicked(gtx) {
		s.closePopovers()
		gtx.Execute(key.FocusCmd{Tag: &s.composer})
		s.conversationList.ScrollToEnd = true
		s.conversationList.Position = layout.Position{}
		gtx.Execute(op.InvalidateCmd{})
	}

	bg := s.theme.surfaceContainerHighest
	border := s.theme.outlineVariant
	fg := s.theme.primary
	if s.jumpToBottomButton.Hovered() {
		bg = s.theme.primaryContainer
		fg = s.theme.onPrimaryContainer
		border = s.theme.primary
	}

	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("Jump to bottom of conversation").Add(gtx.Ops)

	return s.jumpToBottomButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeFull, bg, border, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutActionIcon(gtx, iconChevronDown, 14, fg)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Jump to bottom", textLabelSmall, font.SemiBold, fg, 1)
						})
					}),
				)
			})
		})
	})
}

func (s *shell) layoutMessageCopyButton(gtx layout.Context, keyStr, text string) layout.Dimensions {
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
	btnColor := s.theme.onSurfaceVariant
	if copied {
		btnText = "✓ Copied"
		btnColor = s.theme.onSuccessContainer
	} else if btn.Hovered() {
		btnColor = s.theme.primary
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if copied {
						return s.layoutActionIcon(gtx, iconCheck, 12, btnColor)
					}
					return s.layoutActionIcon(gtx, iconCopy, 12, btnColor)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, btnText, textLabelSmall, font.Medium, btnColor, 1)
					})
				}),
			)
		})
	})
}

func (s *shell) layoutUserRetryButton(gtx layout.Context, keyStr, text string) layout.Dimensions {
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
		s.composer.SetCaret(len(text), len(text))
		s.composerError = ""
		gtx.Execute(key.FocusCmd{Tag: &s.composer})
	}

	btnColor := s.theme.onSurfaceVariant
	if btn.Hovered() {
		btnColor = s.theme.primary
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutActionIcon(gtx, iconCompose, 12, btnColor)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "Retry", textLabelSmall, font.Medium, btnColor, 1)
					})
				}),
			)
		})
	})
}

// conversationButtonKey derives a stable widget key from timeline identity so
// copy/retry button state survives retention trimming.
func conversationButtonKey(sessionID, role string, index int, item desktopstate.TimelineItem) string {
	if item.ID != "" {
		return sessionID + ":" + role + ":" + item.ID
	}
	return sessionID + ":" + role + ":#" + fmt.Sprint(index)
}

func (s *shell) layoutTimelineItem(gtx layout.Context, sessionID string, index int, item desktopstate.TimelineItem) layout.Dimensions {
	descriptionItem := item
	if item.Streaming {
		descriptionItem.Text = streamingText(item.Text)
	}
	itemKey := makeConversationCacheKey(sessionID, index, item)

	if item.Kind == desktopstate.TimelineUser {
		msgKey := conversationButtonKey(sessionID, "user", index, item)
		return desktopInset{Top: 8, Bottom: 8, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
					return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 8, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "You", textLabelSmall, font.SemiBold, s.theme.primary, 1)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Left: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutUserRetryButton(gtx, msgKey, item.Text)
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutMessageCopyButton(gtx, msgKey, item.Text)
											})
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, item.Text, textBodyMedium, font.Normal, s.theme.onSurface, 0)
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
		return s.layoutToolItem(gtx, sessionID, index, item, s.theme.onSurface)
	}

	if item.Kind == desktopstate.TimelinePermission {
		return s.layoutPermissionAuditItem(gtx, item)
	}

	if item.Kind == desktopstate.TimelineStatus {
		return desktopInset{Top: 6, Bottom: 6, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.roundedSurface(gtx, shapeMedium, s.theme.errorContainer, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(conversationItemDescription(descriptionItem)).Add(gtx.Ops)
				return desktopInset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, item.Text, textBodyMedium, font.Medium, s.theme.onErrorContainer, 0)
				})
			})
		})
	}

	// Assistant message
	key := makeConversationCacheKey(sessionID, index, item)
	parsed := s.parsedThinking(key, item.Text)
	foreground := s.theme.onSurface

	return desktopInset{Top: 8, Bottom: 12, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.DescriptionOp(s.conversationDescription(key, descriptionItem)).Add(gtx.Ops)
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				msgKey := conversationButtonKey(sessionID, "asst", index, item)
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutActionIcon(gtx, iconBrandLogo, 16, s.theme.primary)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Protonman", textLabelLarge, font.Bold, s.theme.primary, 1)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedSurface(gtx, shapeSmall, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "AI", textLabelSmall, font.SemiBold, s.theme.onPrimaryContainer, 1)
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
				return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutThinkingBlock(gtx, key, parsed)
				})
			}))
		}

		if parsed.responseText != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
				return desktopInset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Thinking and reasoning…", textLabelMedium, font.Normal, s.theme.onSurfaceVariant, 1)
				})
			}))
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
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
					return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
		s.admitExpansionKey(key)
		buttons = new(conversationDisclosureButtons)
		s.conversationExpandButtons[key] = buttons
	}
	return buttons
}

// admitExpansionKey registers a key in the shared expansion-order FIFO. When
// the bound is reached the oldest key is evicted from every expansion-state
// map at once; the previous per-map evictions could orphan another map's
// entry and grow past the bound.
func (s *shell) admitExpansionKey(key conversationCacheKey) {
	if oldest, _, evicted := s.expansionOrder.put(key, struct{}{}); evicted {
		delete(s.conversationExpandButtons, oldest)
		delete(s.toolOutputPages, oldest)
		delete(s.conversationExpanded, oldest)
		delete(s.conversationPage, oldest)
	}
}

func largeMessageWindow(source string, page int) (string, int, int) {
	pageCount := (len(source) + messagePageBytes - 1) / messagePageBytes
	if page < 0 {
		page = 0
	} else if page >= pageCount {
		page = pageCount - 1
	}
	start := page * messagePageBytes
	for start > 0 && start < len(source) && !utf8.RuneStart(source[start]) {
		start--
	}
	end := min((page+1)*messagePageBytes, len(source))
	for end > start && end < len(source) && !utf8.RuneStart(source[end]) {
		end--
	}
	return strings.Clone(source[start:end]), start, end
}

func largeMessagePreview(source string) string {
	if len(source) <= maxMessagePreviewBytes {
		return source
	}
	end := maxMessagePreviewBytes
	for end > 0 && !utf8.RuneStart(source[end]) {
		end--
	}
	// Substring without allocating a fresh buffer: largeMessagePreview sits on
	// the per-frame render path, and truncation is already conveyed by the
	// "Show full message" affordance that follows the preview.
	return source[:end]
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

func isDiffText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "diff --git ") {
		return true
	}
	lines := strings.Split(text, "\n")
	hasHunk := false
	hasAddDel := false
	for _, l := range lines {
		tl := strings.TrimSpace(l)
		if strings.HasPrefix(tl, "diff --git ") {
			return true
		}
		if strings.HasPrefix(tl, "@@") && strings.Contains(tl[2:], "@@") {
			hasHunk = true
		}
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			hasAddDel = true
		}
		if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			hasAddDel = true
		}
	}
	return hasHunk && hasAddDel
}

func extractDiffFilename(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "diff --git ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 4 {
				return strings.TrimPrefix(parts[3], "b/")
			}
		}
		if strings.HasPrefix(trimmed, "+++ b/") {
			return strings.TrimPrefix(trimmed, "+++ b/")
		}
	}
	return ""
}

func toolCategoryIcon(title string) iconKind {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "bash") || strings.Contains(lower, "exec") || strings.Contains(lower, "command") || strings.Contains(lower, "terminal"):
		return iconTerminal
	case strings.Contains(lower, "read") || strings.Contains(lower, "file") || strings.Contains(lower, "dir") || strings.Contains(lower, "ls"):
		return iconFolder
	case strings.Contains(lower, "grep") || strings.Contains(lower, "find") || strings.Contains(lower, "search"):
		return iconSearch
	case strings.Contains(lower, "edit") || strings.Contains(lower, "write") || strings.Contains(lower, "patch") || strings.Contains(lower, "replace"):
		return iconCompose
	default:
		return iconSettings
	}
}

// toolDiffInfo classifies and parses a tool item's text at most once per source.
// The conversation view re-runs for every visible item on each frame, so the full
// line split lives here instead of on the render hot path.
func (s *shell) toolDiffInfo(toolKey, text string) toolDiffCacheEntry {
	if cached, ok := s.toolDiffCache.get(toolKey); ok && cached.source == text {
		return cached
	}
	entry := toolDiffCacheEntry{source: text}
	if isDiffText(text) {
		entry.isDiff = true
		entry.filename = extractDiffFilename(text)
		entry.additions, entry.deletions = diffutil.DiffStats(text)
		entry.preview, entry.omitted = diffutil.ExtractPreview(text, 32)
	}
	s.toolDiffCache.put(toolKey, entry)
	return entry
}

func (s *shell) layoutToolItem(gtx layout.Context, sessionID string, index int, item desktopstate.TimelineItem, foreground color.NRGBA) layout.Dimensions {
	toolKey := item.ID
	if toolKey == "" {
		toolKey = fmt.Sprintf("%s:tool:%d", sessionID, index)
	}
	cacheKey := makeConversationCacheKey(sessionID, index, item)

	if s.toolExpandButtons == nil {
		s.toolExpandButtons = make(map[string]*widget.Clickable)
	}
	btn, ok := s.toolExpandButtons[toolKey]
	if !ok {
		btn = new(widget.Clickable)
		s.toolExpandButtons[toolKey] = btn
	}

	if s.toolExpanded == nil {
		s.toolExpanded = make(map[string]bool)
	}
	if btn.Clicked(gtx) {
		s.toolExpanded[toolKey] = !s.toolExpanded[toolKey]
	}

	status := strings.TrimSpace(item.Status)
	statusBg := s.theme.secondaryContainer
	statusFg := s.theme.onSecondaryContainer
	statusLabel := "TOOL"
	isFailed := false
	isRunning := false
	if status != "" {
		statusLabel = strings.ToUpper(status)
		switch strings.ToLower(status) {
		case "completed", "success", "ok":
			statusBg = s.theme.successContainer
			statusFg = s.theme.onSuccessContainer
		case "running", "in_progress":
			statusBg = s.theme.primaryContainer
			statusFg = s.theme.onPrimaryContainer
			isRunning = true
		case "failed", "error":
			statusBg = s.theme.errorContainer
			statusFg = s.theme.onErrorContainer
			isFailed = true
		}
	}

	if diff := s.toolDiffInfo(toolKey, item.Text); diff.isDiff {
		return s.layoutDiffToolItem(gtx, cacheKey, toolKey, item, diff, statusLabel, statusBg, statusFg)
	}

	expanded, hasExplicit := s.toolExpanded[toolKey]
	if !hasExplicit {
		expanded = isRunning || isFailed
	}

	chevron := iconChevronRight
	if expanded {
		chevron = iconChevronDown
	}

	icon := toolCategoryIcon(item.Title)

	headerBg := s.theme.surfaceContainerHigh
	headerBorder := s.theme.outlineVariant
	if btn.Hovered() {
		headerBg = s.theme.surfaceContainerHighest
		headerBorder = s.theme.primary
	}

	return desktopInset{Top: 4, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(36)
					return s.roundedBorderSurface(gtx, shapeMedium, headerBg, headerBorder, 1, func(gtx layout.Context) layout.Dimensions {
						semantic.DescriptionOp(s.conversationDescription(cacheKey, item)).Add(gtx.Ops)
						return desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, icon, 16, s.theme.primary)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, item.Title, textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, statusBg, func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, statusLabel, textLabelSmall, font.Bold, statusFg, 1)
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutActionIcon(gtx, chevron, 12, s.theme.onSurfaceVariant)
									})
								}),
							)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !expanded || strings.TrimSpace(item.Text) == "" {
					return layout.Dimensions{}
				}
				return s.layoutToolOutputBody(gtx, cacheKey, toolKey, item.Text)
			}),
		)
	})
}

// toolOutputPageButtons bundles the widget state for one paged tool output.
type toolOutputPageButtons struct {
	previous widget.Clickable
	next     widget.Clickable
	collapse widget.Clickable
}

// toolOutputPageState returns the paging widgets for a tool output, creating
// them on first use and evicting an arbitrary entry once the map exceeds the
// shared expansion-key bound.
func (s *shell) toolOutputPageState(key conversationCacheKey) *toolOutputPageButtons {
	if s.toolOutputPages == nil {
		s.toolOutputPages = make(map[conversationCacheKey]*toolOutputPageButtons)
	}
	pages := s.toolOutputPages[key]
	if pages == nil {
		s.admitExpansionKey(key)
		pages = new(toolOutputPageButtons)
		s.toolOutputPages[key] = pages
	}
	return pages
}

// toolOutputWindow returns the visible page text and paging facts for a tool
// output. Pure so pagination behavior is testable without a frame.
func toolOutputWindow(source string, page int) (text string, pageCount, current int) {
	pageCount = (len(source) + messagePageBytes - 1) / messagePageBytes
	if page < 0 {
		page = 0
	} else if page >= pageCount {
		page = pageCount - 1
	}
	start := page * messagePageBytes
	for start > 0 && start < len(source) && !utf8.RuneStart(source[start]) {
		start--
	}
	end := min((page+1)*messagePageBytes, len(source))
	for end > start && end < len(source) && !utf8.RuneStart(source[end]) {
		end--
	}
	return source[start:end], pageCount, page
}

// layoutToolOutputBody renders one expanded tool item's output. Small outputs
// render whole; large ones reuse the assistant large-message pattern: a
// preview plus "Show full message", then byte-paged parts. Per-frame layout
// cost stays bounded by the page size rather than the output size.
func (s *shell) layoutToolOutputBody(gtx layout.Context, key conversationCacheKey, toolKey, text string) layout.Dimensions {
	if len(text) <= maxToolOutputUnpagedBytes {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Spacer{}.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutMessageCopyButton(gtx, toolKey+":copy", text)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, text, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 0)
				})
			}),
		)
	}

	pages := s.toolOutputPageState(key)
	expanded := s.conversationExpanded[key]
	if !expanded {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, largeMessagePreview(text), textBodySmall, font.Normal, s.theme.onSurfaceVariant, largeMessagePreviewLines)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &pages.collapse, "Show full output", true, func() {
					s.conversationExpanded[key] = true
				})
			}),
		)
	}

	page := 0
	if stored, ok := s.conversationPage[key]; ok {
		page = stored
	}
	pageText, pageCount, current := toolOutputWindow(text, page)
	s.conversationPage[key] = current
	copyText := text
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMessageCopyButton(gtx, toolKey+":copy", copyText)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, pageText, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "Part " + strconv.Itoa(current+1) + " of " + strconv.Itoa(pageCount)
			return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, s.theme.onSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutButton(gtx, &pages.previous, "Previous part", current > 0, func() {
						if page, ok := s.conversationPage[key]; ok && page > 0 {
							s.conversationPage[key] = page - 1
						}
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutButton(gtx, &pages.next, "Next part", current < pageCount-1, func() {
						s.conversationPage[key] = current + 1
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutButton(gtx, &pages.collapse, "Show less", true, func() {
							s.conversationExpanded[key] = false
							s.conversationPage[key] = 0
						})
					})
				}),
			)
		}),
	)
}

func (s *shell) layoutDiffToolItem(gtx layout.Context, cacheKey conversationCacheKey, toolKey string, item desktopstate.TimelineItem, diff toolDiffCacheEntry, statusLabel string, statusBg, statusFg color.NRGBA) layout.Dimensions {
	adds, dels := diff.additions, diff.deletions
	displayTitle := item.Title
	if diff.filename != "" {
		displayTitle = diff.filename
	}

	btn, ok := s.toolExpandButtons[toolKey]
	if !ok {
		btn = new(widget.Clickable)
		s.toolExpandButtons[toolKey] = btn
	}
	if btn.Clicked(gtx) {
		s.toolExpanded[toolKey] = !s.toolExpanded[toolKey]
	}

	expanded, hasExplicit := s.toolExpanded[toolKey]
	if !hasExplicit {
		// Diffs default collapsed like plain tool items: expanded previews
		// re-render their line list on every frame while scrolling through
		// history, which dominated frame cost in diff-heavy sessions.
		expanded = false
	}

	chevron := iconChevronRight
	if expanded {
		chevron = iconChevronDown
	}

	headerBg := s.theme.surfaceContainerHigh
	headerBorder := s.theme.outlineVariant
	if btn.Hovered() {
		headerBg = s.theme.surfaceContainerHighest
		headerBorder = s.theme.primary
	}

	previewLines, omitted := diff.preview, diff.omitted

	return desktopInset{Top: 4, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(36)
					return s.roundedBorderSurface(gtx, shapeMedium, headerBg, headerBorder, 1, func(gtx layout.Context) layout.Dimensions {
						semantic.DescriptionOp(s.conversationDescription(cacheKey, item)).Add(gtx.Ops)
						return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, iconCompose, 16, s.theme.primary)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, displayTitle, textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if adds == 0 {
										return layout.Dimensions{}
									}
									return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.diffAddedContainer, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, fmt.Sprintf("+%d", adds), textLabelSmall, font.Bold, s.theme.diffAdded, 1)
											})
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if dels == 0 {
										return layout.Dimensions{}
									}
									return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.diffDeletedContainer, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, fmt.Sprintf("-%d", dels), textLabelSmall, font.Bold, s.theme.diffDeleted, 1)
											})
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, statusBg, func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, statusLabel, textLabelSmall, font.Bold, statusFg, 1)
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutActionIcon(gtx, chevron, 12, s.theme.onSurfaceVariant)
									})
								}),
							)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !expanded || len(previewLines) == 0 {
					return layout.Dimensions{}
				}
				return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lineChildren := make([]layout.FlexChild, 0, len(previewLines)+2)
							lineChildren = append(lineChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.Spacer{}.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutMessageCopyButton(gtx, toolKey+":diffcopy", item.Text)
									}),
								)
							}))
							for _, line := range previewLines {
								lineStr := line
								lineChildren = append(lineChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutDiffLine(gtx, lineStr)
								}))
							}
							if omitted > 0 {
								lineChildren = append(lineChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, fmt.Sprintf("… %d more lines omitted", omitted), textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
									})
								}))
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lineChildren...)
						})
					})
				})
			}),
		)
	})
}

func (s *shell) layoutDiffLine(gtx layout.Context, line string) layout.Dimensions {
	lineColor := s.theme.onSurfaceVariant
	lineBg := color.NRGBA{}
	trimmed := strings.TrimSpace(line)

	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		lineColor = s.theme.diffAdded
		lineBg = s.theme.diffAddedContainer
	} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		lineColor = s.theme.diffDeleted
		lineBg = s.theme.diffDeletedContainer
	} else if strings.HasPrefix(trimmed, "@@") {
		lineColor = s.theme.tertiary
	}

	content := func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 1, Bottom: 1, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, line, textBodySmall, font.Normal, lineColor, 1)
		})
	}

	if lineBg.A > 0 {
		return s.roundedSurface(gtx, shapeSmall, lineBg, content)
	}
	return content(gtx)
}

func (s *shell) layoutSubagentItem(gtx layout.Context, subagent desktopstate.SubagentState) layout.Dimensions {
	profile := strings.ToLower(strings.TrimSpace(subagent.Profile))
	badgeText := "SUBAGENT"
	badgeBg := s.theme.secondaryContainer
	badgeFg := s.theme.onSecondaryContainer
	accentColor := s.theme.secondary

	switch profile {
	case "strength", "str":
		badgeText = "STR · STRENGTH"
		badgeBg = s.theme.strengthContainer
		badgeFg = s.theme.onStrengthContainer
		accentColor = s.theme.strength
	case "agility", "agi":
		badgeText = "AGI · AGILITY"
		badgeBg = s.theme.agilityContainer
		badgeFg = s.theme.onAgilityContainer
		accentColor = s.theme.agility
	case "intelligence", "int":
		badgeText = "INT · INTELLIGENCE"
		badgeBg = s.theme.intelligenceContainer
		badgeFg = s.theme.onIntelligenceContainer
		accentColor = s.theme.intelligence
	default:
		if profile != "" {
			badgeText = "" + strings.ToUpper(profile)
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

	return desktopInset{Top: 6, Bottom: 6, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp("Delegated agent " + subagent.Profile + ", " + subagent.Status + ", " + subagent.Task).Add(gtx.Ops)
			return layout.Stack{Alignment: layout.W}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 12, Bottom: 12, Left: 18, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, badgeBg, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, badgeText, textLabelMedium, font.Bold, badgeFg, 1)
											})
										})
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.Spacer{}.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, statusBg, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
								return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, subagent.Task, textBodyMedium, font.Medium, s.theme.onSurface, 3)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if strings.TrimSpace(subagent.Summary) == "" {
									return layout.Dimensions{}
								}
								return desktopInset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, subagent.Summary, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 6)
										})
									})
								})
							}),
						)
					})
				}),
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					stripeWidth := gtx.Dp(4)
					cornerRadius := gtx.Dp(shapeLarge)
					stack := clip.RRect{
						Rect: image.Rectangle{Max: image.Point{X: stripeWidth, Y: gtx.Constraints.Max.Y}},
						NW:   cornerRadius,
						SW:   cornerRadius,
					}.Push(gtx.Ops)
					paint.Fill(gtx.Ops, accentColor)
					stack.Pop()
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
			)
		})
	})
}

func (s *shell) layoutMarkdown(gtx layout.Context, key conversationCacheKey, source string, fallback color.NRGBA) layout.Dimensions {
	if strings.TrimSpace(source) == "" {
		s.dropMarkdownCache(key)
		return layout.Dimensions{}
	}
	if len(source) > maxMarkdownRenderBytes {
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

func (s *shell) responseBlocks(key conversationCacheKey, source string) []markdownBlock {
	if cached, ok := s.conversationResponseCache[key]; ok && cached.source == source {
		return cached.blocks
	}
	blocks := splitMarkdownCodeBlocks(source)
	if source == "" || len(source) > maxResponseSplitSourceBytes {
		return blocks
	}
	if s.conversationResponseCache == nil {
		s.conversationResponseCache = make(map[conversationCacheKey]conversationResponseCache)
	}
	entryBytes := responseSplitRetainedEstimate + len(source) + len(blocks)*16
	if entryBytes > maxResponseSplitCacheBytes {
		return blocks
	}
	if previous, ok := s.conversationResponseCache[key]; ok {
		s.conversationResponseBytes -= responseSplitRetainedEstimate + len(previous.source) + len(previous.blocks)*16
		delete(s.conversationResponseCache, key)
		for index, id := range s.conversationResponseOrder {
			if id == key {
				s.conversationResponseOrder = append(s.conversationResponseOrder[:index], s.conversationResponseOrder[index+1:]...)
				break
			}
		}
	}
	for len(s.conversationResponseCache) >= maxResponseSplitCacheEntries || s.conversationResponseBytes+entryBytes > maxResponseSplitCacheBytes {
		if len(s.conversationResponseOrder) == 0 {
			break
		}
		oldest := s.conversationResponseOrder[0]
		s.conversationResponseOrder = s.conversationResponseOrder[1:]
		if previous, ok := s.conversationResponseCache[oldest]; ok {
			delete(s.conversationResponseCache, oldest)
			s.conversationResponseBytes -= responseSplitRetainedEstimate + len(previous.source) + len(previous.blocks)*16
		}
	}
	s.conversationResponseCache[key] = conversationResponseCache{source: source, blocks: blocks}
	s.conversationResponseOrder = append(s.conversationResponseOrder, key)
	s.conversationResponseBytes += entryBytes
	return blocks
}

func (s *shell) layoutRichResponse(gtx layout.Context, key conversationCacheKey, blocks []markdownBlock, foreground color.NRGBA) layout.Dimensions {
	if len(blocks) == 0 {
		return layout.Dimensions{}
	}
	if len(blocks) == 1 && blocks[0].kind == markdownBlockText {
		return s.layoutMarkdown(gtx, key, blocks[0].text, foreground)
	}

	blockChildren := make([]layout.FlexChild, 0, len(blocks))
	for idx, b := range blocks {
		blockIdx := idx
		block := b
		subKey := conversationCacheKey{
			sessionID: key.sessionID,
			itemID:    fmt.Sprintf("%s-b%d", key.itemID, blockIdx),
		}
		if block.kind == markdownBlockCode {
			blockChildren = append(blockChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 6, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutCodeCard(gtx, subKey, block.lang, block.code)
				})
			}))
		} else {
			if strings.TrimSpace(block.text) != "" {
				blockChildren = append(blockChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMarkdown(gtx, subKey, block.text, foreground)
				}))
			}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, blockChildren...)
}

func (s *shell) layoutCodeCard(gtx layout.Context, key conversationCacheKey, lang, code string) layout.Dimensions {
	keyStr := key.sessionID + ":" + key.itemID
	copyBtn, ok := s.codeCopyButtons[keyStr]
	if !ok {
		copyBtn = new(widget.Clickable)
		if s.codeCopyButtons == nil {
			s.codeCopyButtons = make(map[string]*widget.Clickable)
		}
		s.codeCopyButtons[keyStr] = copyBtn
	}

	if copyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(code)),
		})
		if s.codeCopiedAt == nil {
			s.codeCopiedAt = make(map[string]time.Time)
		}
		s.codeCopiedAt[keyStr] = time.Now()
	}

	copied := false
	if s.codeCopiedAt != nil {
		if t, ok := s.codeCopiedAt[keyStr]; ok && time.Since(t) < 2*time.Second {
			copied = true
		}
	}

	displayLang := strings.ToUpper(strings.TrimSpace(lang))
	if displayLang == "" {
		displayLang = "CODE"
	}

	return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 4, Bottom: 4, Left: 10, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, displayLang, textLabelSmall, font.Bold, s.theme.onSurfaceVariant, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btnText := "Copy"
								btnColor := s.theme.onSurfaceVariant
								if copied {
									btnText = "✓ Copied"
									btnColor = s.theme.onSuccessContainer
								}
								return copyBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, btnText, textLabelSmall, font.Medium, btnColor, 1)
									})
								})
							}),
						)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					spans := s.cachedCodeSpans(key, lang, code)
					return richtext.Text(nil, s.theme.material.Shaper, spans...).Layout(gtx)
				})
			}),
		)
	})
}

func codeCacheEntryBytes(key conversationCacheKey, cached conversationCodeCache) int {
	bytes := 128 + len(key.sessionID) + len(key.itemID) + len(cached.lang) + len(cached.source)
	for _, span := range cached.spans {
		bytes += markdownSpanRetainedEstimate + len(span.Content)
	}
	return bytes
}

func (s *shell) cachedCodeSpans(key conversationCacheKey, lang, code string) []richtext.SpanStyle {
	if cached, ok := s.conversationCodeCache[key]; ok && cached.source == code && cached.lang == lang {
		return cached.spans
	}
	spans := highlightCodeSpans(s.theme, lang, code)
	entry := conversationCodeCache{source: code, lang: lang, spans: spans}
	entry.bytes = codeCacheEntryBytes(key, entry)
	if entry.bytes > maxConversationCacheBytes {
		return spans
	}
	if s.conversationCodeCache == nil {
		s.conversationCodeCache = make(map[conversationCacheKey]conversationCodeCache)
	}
	if _, ok := s.conversationCodeCache[key]; ok {
		for index, id := range s.conversationCodeCacheOrder {
			if id == key {
				s.conversationCodeCacheOrder = append(s.conversationCodeCacheOrder[:index], s.conversationCodeCacheOrder[index+1:]...)
				break
			}
		}
		if previous, ok := s.conversationCodeCache[key]; ok {
			s.conversationCodeCacheBytes -= previous.bytes
		}
		delete(s.conversationCodeCache, key)
	}
	for len(s.conversationCodeCache) >= maxConversationCacheEntries || s.conversationCodeCacheBytes+entry.bytes > maxConversationCacheBytes {
		if len(s.conversationCodeCacheOrder) == 0 {
			break
		}
		oldest := s.conversationCodeCacheOrder[0]
		s.conversationCodeCacheOrder = s.conversationCodeCacheOrder[1:]
		if previous, ok := s.conversationCodeCache[oldest]; ok {
			delete(s.conversationCodeCache, oldest)
			s.conversationCodeCacheBytes -= previous.bytes
		}
	}
	s.conversationCodeCache[key] = entry
	s.conversationCodeCacheOrder = append(s.conversationCodeCacheOrder, key)
	s.conversationCodeCacheBytes += entry.bytes
	return spans
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
		for index, id := range s.conversationCacheOrder {
			if id == key {
				s.conversationCacheOrder = append(s.conversationCacheOrder[:index], s.conversationCacheOrder[index+1:]...)
				break
			}
		}
	}
	for len(s.conversationCache) >= maxConversationCacheEntries || s.conversationCacheBytes+cached.bytes > maxConversationCacheBytes {
		if len(s.conversationCacheOrder) == 0 {
			break
		}
		oldest := s.conversationCacheOrder[0]
		s.conversationCacheOrder = s.conversationCacheOrder[1:]
		if previous, ok := s.conversationCache[oldest]; ok {
			delete(s.conversationCache, oldest)
			s.conversationCacheBytes -= previous.bytes
		}
	}
	s.conversationCache[key] = cached
	s.conversationCacheOrder = append(s.conversationCacheOrder, key)
	s.conversationCacheBytes += cached.bytes
	return true
}

func (s *shell) dropMarkdownCache(key conversationCacheKey) {
	if cached, ok := s.conversationCache[key]; ok {
		delete(s.conversationCache, key)
		s.conversationCacheBytes -= cached.bytes
	}
	for index, id := range s.conversationCacheOrder {
		if id == key {
			s.conversationCacheOrder = append(s.conversationCacheOrder[:index], s.conversationCacheOrder[index+1:]...)
			break
		}
	}
}

func (s *shell) layoutPermissionAuditItem(gtx layout.Context, item desktopstate.TimelineItem) layout.Dimensions {
	isAllow := strings.HasPrefix(item.Status, "Allowed") || item.Status == "answered"
	isDeny := strings.HasPrefix(item.Status, "Denied") || item.Status == "declined"

	accentColor := s.theme.primary
	icon := iconCheck
	if isAllow {
		accentColor = s.theme.onSuccessContainer
		icon = iconCheck
	} else if isDeny {
		accentColor = s.theme.onErrorContainer
		icon = iconClose
	}

	return desktopInset{Top: 4, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutActionIcon(gtx, icon, 14, accentColor)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, item.Title, textLabelMedium, font.SemiBold, s.theme.onSurface, 1)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if strings.TrimSpace(item.Text) == "" {
							return layout.Dimensions{}
						}
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, item.Text, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 1)
						})
					}),
				)
			})
		})
	})
}

func (s *shell) layoutPermissionPanel(gtx layout.Context, state desktopstate.State, request desktopstate.PermissionRequest) layout.Dimensions {
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
						s.onResolvePermission(request.RequestID, opt.ID)
						break
					}
				}
			} else if e.Name == key.NameEscape {
				for _, opt := range request.Options {
					optLower := strings.ToLower(opt.ID)
					if strings.Contains(optLower, "reject") || strings.Contains(optLower, "deny") {
						s.onResolvePermission(request.RequestID, opt.ID)
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
	containerBg := s.theme.surfaceContainerHigh
	borderColor := s.theme.outlineVariant
	borderWidth := 1
	if isElevatedRisk {
		containerBg = s.theme.warningContainer
		borderColor = s.theme.onErrorContainer
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
		return desktopInset{Top: 12, Bottom: 12, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "⚡ "+toolLabel, textLabelSmall, font.Bold, s.theme.onPrimaryContainer, 1)
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, request.Title, textTitleSmall, font.SemiBold, s.theme.onSurface, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if qTotal <= 1 {
										return layout.Dimensions{}
									}
									counterText := fmt.Sprintf("(%d of %d)", qIdx, qTotal)
									return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, counterText, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
									})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if isElevatedRisk {
								return s.roundedSurface(gtx, shapeSmall, s.theme.errorContainer, func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, "⚠️ Elevated Risk", textLabelSmall, font.Bold, s.theme.onErrorContainer, 1)
									})
								})
							}
							return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Permission Required", textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
								})
							})
						}),
					)
				}),
			}

			if displayCmd != "" {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 8, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, displayCmd, textBodyMedium, font.Normal, s.theme.onSurface, 6)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if s.permissionCopyButtons[request.RequestID] == nil {
											s.permissionCopyButtons[request.RequestID] = new(widget.Clickable)
										}
										copyBtn := s.permissionCopyButtons[request.RequestID]
										return s.layoutIconActionButton(gtx, copyBtn, iconCopy, "Copy command", func() {
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

					return desktopInset{Top: 2, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, toggleBtn, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, toggleLabel, textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !expanded {
									return layout.Dimensions{}
								}
								return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, request.RawJSON, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 16)
										})
									})
								})
							}),
						)
					})
				}))
			}

			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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

func permissionOptionChildren(s *shell, request desktopstate.PermissionRequest) []layout.FlexChild {
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
			return desktopUniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if strings.Contains(optLower, "reject") || strings.Contains(optLower, "deny") {
					label := option.Name + " (Esc)"
					return s.layoutDangerButton(gtx, btn, label, true, func() {
						s.onResolvePermission(request.RequestID, option.ID)
					})
				}
				if strings.Contains(optLower, "session") || strings.Contains(optLower, "always") {
					return s.layoutButton(gtx, btn, option.Name, true, func() {
						s.onResolvePermission(request.RequestID, option.ID)
					})
				}
				label := option.Name + " (↵)"
				return s.layoutPrimaryButton(gtx, btn, label, true, func() {
					s.onResolvePermission(request.RequestID, option.ID)
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

func (s *shell) layoutQuestionPanel(gtx layout.Context, state desktopstate.State, request desktopstate.QuestionRequest) layout.Dimensions {
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
				if s.onResolveQuestion != nil {
					s.onResolveQuestion(request.RequestID, desktopstate.QuestionResponse{
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
					if s.onResolveQuestion != nil {
						s.onResolveQuestion(request.RequestID, desktopstate.QuestionResponse{
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

	return s.roundedSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 14, Bottom: 14, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "💬 Question from Agent", textTitleMedium, font.SemiBold, s.theme.primary, 1)
						}),
					)
				}),
			}

			for _, q := range request.Questions {
				q := q
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 6, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, q.Question, textBodyMedium, font.Medium, s.theme.onSurface, 4)
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
									return desktopUniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										bg := s.theme.surfaceContainerLow
										fg := s.theme.onSurface
										borderCol := s.theme.outlineVariant
										if selected {
											bg = s.theme.primaryContainer
											fg = s.theme.onPrimaryContainer
											borderCol = s.theme.primary
										}
										return s.roundedBorderSurface(gtx, shapeSmall, bg, borderCol, 1, func(gtx layout.Context) layout.Dimensions {
											return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
												return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
														layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															label := glyph + hotkeyHint + opt
															return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, fg, 2)
														}),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															if !isRecommended {
																return layout.Dimensions{}
															}
															return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return s.roundedSurface(gtx, shapeSmall, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
																	return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																		return s.layoutLabel(gtx, "★ Recommended", textLabelSmall, font.SemiBold, s.theme.onPrimaryContainer, 1)
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
					return desktopInset{Top: 6, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								ed := material.Editor(s.theme.material, &qs.customAnswer, "Or type custom answer here…")
								ed.TextSize = unit.Sp(13)
								return ed.Layout(gtx)
							})
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutButton(gtx, &qs.declineButton, "Decline (Esc)", true, func() {
									delete(s.questionStates, request.RequestID)
									if s.onResolveQuestion != nil {
										s.onResolveQuestion(request.RequestID, desktopstate.QuestionResponse{
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
								if s.onResolveQuestion != nil {
									s.onResolveQuestion(request.RequestID, desktopstate.QuestionResponse{
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

func (s *shell) layoutComposer(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	busy := sessionBusy(session.Status)
	connected := sessionConnection(snapshot, session.AgentID) == connectionConnected
	loading := snapshot.HistoryState == historyStateLoading
	canSend := connected && !busy && !loading
	canEdit := canSend && !s.settingsModalOpen
	compact := gtx.Constraints.Max.X < gtx.Dp(480)
	helper := composerHelper(compact, busy, loading, connected)
	s.activeWorkspace = session.Workspace
	s.composer.ReadOnly = !canEdit
	if canEdit {
		s.refreshMentionState(session.Workspace)
		if s.mentionActive && len(s.mentionItems) > 0 {
			for {
				evt, ok := gtx.Event(key.Filter{Focus: &s.composer, Name: key.NameUpArrow})
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
				evt, ok := gtx.Event(key.Filter{Focus: &s.composer, Name: key.NameDownArrow})
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
				evt, ok := gtx.Event(key.Filter{Focus: &s.composer, Name: key.NameTab})
				if !ok {
					break
				}
				if e, ok := evt.(key.Event); ok && e.State == key.Press {
					s.applySelectedMention()
				}
			}
			for {
				evt, ok := gtx.Event(key.Filter{Focus: &s.composer, Name: key.NameEscape})
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
			event, ok := s.composer.Update(gtx)
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

	outerInset := desktopInset{Top: 6, Bottom: 16, Left: 16, Right: 16}
	if compact {
		outerInset = desktopInset{Top: 4, Bottom: 8, Left: 8, Right: 8}
	}

	helperColor := s.theme.onSurfaceVariant
	if s.composerError != "" {
		helper = s.composerError
		helperColor = s.theme.onErrorContainer
	}

	borderColor := s.theme.outlineVariant
	borderWidth := 1
	if gtx.Focused(&s.composer) {
		borderColor = s.theme.primary
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
			} else if s.modelPopoverVisible {
				containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutModelPopover(gtx, session, snapshot, canSend)
				}))
			} else if s.reasoningPopoverVisible {
				containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutReasoningPopover(gtx, session, canSend)
				}))
			} else if s.permissionModePopoverVisible {
				containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutPermissionModePopover(gtx, session, canSend)
				}))
			}

			containerChildren = append(containerChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.roundedBorderSurface(gtx, shapeExtraLarge, s.theme.surfaceContainer, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
					innerInset := desktopInset{Top: 10, Bottom: 10, Left: 14, Right: 14}
					if compact {
						innerInset = desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}
					}
					return innerInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutComposerContextChips(gtx, session, snapshot, canSend)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutComposerEditor(gtx, canSend, compact)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											if helper == "" {
												return layout.Spacer{}.Layout(gtx)
											}
											return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, helper, textLabelSmall, font.Normal, helperColor, 1)
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if busy {
												return s.layoutComposerActionButton(gtx, &s.stopButton, "Stop", "Stop prompt", connected, true, s.onCancelPrompt)
											}
											label := "↑"
											if !compact {
												label = "Send"
											}
											return s.layoutComposerActionButton(gtx, &s.sendButton, label, "Send prompt", canSend && s.composerHasContent, false, func() {
												s.submitComposer(s.composer.Text())
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

func (s *shell) layoutComposerActionButton(gtx layout.Context, button *widget.Clickable, icon, tooltip string, enabled, danger bool, action func()) layout.Dimensions {
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
	bg := s.theme.primary
	fg := s.theme.onPrimary
	if danger {
		bg = s.theme.errorContainer
		fg = s.theme.onErrorContainer
	}
	if !gtx.Enabled() {
		bg = s.theme.surfaceContainerHigh
		fg = s.theme.onSurfaceVariant
	} else if button.Hovered() && !danger {
		bg = s.theme.primaryContainer
		fg = s.theme.onPrimaryContainer
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeFull, bg, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				kind := iconSend
				if icon == "Stop" {
					kind = iconStop
				}
				if icon == "↑" || icon == "Stop" {
					return s.layoutActionIcon(gtx, kind, 16, fg)
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.layoutActionIcon(gtx, kind, 16, fg) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, icon, textTitleMedium, font.Bold, fg, 1)
						})
					}),
				)
			}))
		})
	})
	if enabled && gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeFull, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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

func (s *shell) layoutComposerContextChips(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot, enabled bool) layout.Dimensions {
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
		agentID = controllerAgentID
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
			return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeMedium, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "Goal: "+compactInspectorText(goal, goalLimit), textLabelSmall, font.Medium, s.theme.onPrimaryContainer, 1)
					})
				})
			})
		}))
	}

	chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if enabled && s.modelChipButton.Clicked(gtx) {
			if s.modelPopoverVisible {
				s.closePopovers()
				gtx.Execute(key.FocusCmd{Tag: &s.composer})
			} else {
				s.openModelPopover()
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		semantic.DescriptionOp("Change model (Alt+M)").Add(gtx.Ops)
		bg := s.theme.secondaryContainer
		fg := s.theme.onSecondaryContainer
		if s.modelPopoverVisible {
			bg = s.theme.primaryContainer
			fg = s.theme.onPrimaryContainer
		} else if enabled && s.modelChipButton.Hovered() {
			bg = s.theme.surfaceContainerHighest
		}
		chipGtx := gtx
		if !enabled {
			chipGtx = chipGtx.Disabled()
		}
		chevronKind := iconChevronDown
		if s.modelPopoverVisible {
			chevronKind = iconChevronUp
		}
		return desktopInset{Right: 6}.Layout(chipGtx, func(gtx layout.Context) layout.Dimensions {
			return s.modelChipButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := s.roundedSurface(gtx, shapeMedium, bg, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Model: "+compactInspectorText(model, modelLimit), textLabelSmall, font.Medium, fg, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, chevronKind, 10, fg)
								})
							}),
						)
					})
				})
				if s.modelPopoverVisible {
					widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: dims.Size}
					})
				}
				return dims
			})
		})
	}))

	chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if enabled && s.reasoningChipButton.Clicked(gtx) {
			if s.reasoningPopoverVisible {
				s.closePopovers()
				gtx.Execute(key.FocusCmd{Tag: &s.composer})
			} else {
				s.openReasoningPopover()
				gtx.Execute(key.FocusCmd{Tag: &s.composer})
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		semantic.DescriptionOp("Change reasoning effort (Alt+R)").Add(gtx.Ops)
		bg := s.theme.tertiaryContainer
		fg := s.theme.onTertiaryContainer
		if s.reasoningPopoverVisible {
			bg = s.theme.primaryContainer
			fg = s.theme.onPrimaryContainer
		} else if enabled && s.reasoningChipButton.Hovered() {
			bg = s.theme.surfaceContainerHighest
		}
		chipGtx := gtx
		if !enabled {
			chipGtx = chipGtx.Disabled()
		}
		chevronKind := iconChevronDown
		if s.reasoningPopoverVisible {
			chevronKind = iconChevronUp
		}
		return desktopInset{Right: 6}.Layout(chipGtx, func(gtx layout.Context) layout.Dimensions {
			return s.reasoningChipButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := s.roundedSurface(gtx, shapeMedium, bg, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, reasoningLabel+reasoning, textLabelSmall, font.Medium, fg, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, chevronKind, 10, fg)
								})
							}),
						)
					})
				})
				if s.reasoningPopoverVisible {
					widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: dims.Size}
					})
				}
				return dims
			})
		})
	}))

	chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if enabled && s.permissionModeChipButton.Clicked(gtx) {
			if s.permissionModePopoverVisible {
				s.closePopovers()
				gtx.Execute(key.FocusCmd{Tag: &s.composer})
			} else {
				s.openPermissionModePopover()
				gtx.Execute(key.FocusCmd{Tag: &s.composer})
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		semantic.DescriptionOp("Change permission mode (Alt+P)").Add(gtx.Ops)
		bg := s.theme.surfaceContainerHigh
		fg := s.theme.onSurface
		switch mode {
		case "plan", "deny":
			bg = s.theme.secondaryContainer
			fg = s.theme.onSecondaryContainer
		case "always-approve", "auto":
			bg = s.theme.warningContainer
			fg = s.theme.onWarningContainer
		}
		if s.permissionModePopoverVisible {
			bg = s.theme.primaryContainer
			fg = s.theme.onPrimaryContainer
		} else if enabled && s.permissionModeChipButton.Hovered() {
			bg = s.theme.surfaceContainerHighest
		}
		chipGtx := gtx
		if !enabled {
			chipGtx = chipGtx.Disabled()
		}
		chevronKind := iconChevronDown
		if s.permissionModePopoverVisible {
			chevronKind = iconChevronUp
		}
		return desktopInset{Right: 6}.Layout(chipGtx, func(gtx layout.Context) layout.Dimensions {
			return s.permissionModeChipButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := s.roundedSurface(gtx, shapeMedium, bg, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, modeLabel+displayMode, textLabelSmall, font.Medium, fg, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, chevronKind, 10, fg)
								})
							}),
						)
					})
				})
				if s.permissionModePopoverVisible {
					widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: dims.Size}
					})
				}
				return dims
			})
		})
	}))

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, chips...)
}

func (s *shell) layoutComposerEditor(gtx layout.Context, enabled, compact bool) layout.Dimensions {
	minHeight, maxHeight, verticalInset := gtx.Dp(44), gtx.Dp(180), unit.Dp(8)
	if compact {
		minHeight, maxHeight, verticalInset = gtx.Dp(40), gtx.Dp(140), unit.Dp(6)
	}
	gtx.Constraints.Min.Y = minHeight
	gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, maxHeight)
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
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
	return desktopInset{Top: verticalInset, Bottom: verticalInset, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.StackChild{
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				semantic.DescriptionOp("Message Protonman. Press Enter to send; Shift+Enter to insert a new line.").Add(gtx.Ops)
				return s.composer.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(font.Normal), textBodyMedium, textCall, selectionCall)
			}),
		}
		if strings.TrimSpace(s.composer.Text()) == "" && !gtx.Focused(&s.composer) {
			children = append(children, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Message Protonman…", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 1)
			}))
		}
		return layout.Stack{Alignment: layout.NW}.Layout(gtx, children...)
	})
}

func (s *shell) submitComposer(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	expanded, err := expandMentions(text, s.activeWorkspace)
	if err != nil {
		s.composerError = err.Error()
		return
	}
	s.composerError = ""
	s.onSendPrompt(expanded)
	s.setComposerText("")
	s.forgetComposerDraft(s.activeSessionID)
	s.conversationList.ScrollToEnd = true
	s.conversationList.Position = layout.Position{}
}

func (s *shell) rememberComposerDraft(sessionID, draft string) {
	if sessionID == "" || strings.TrimSpace(draft) == "" {
		return
	}
	if s.composerDrafts == nil {
		s.composerDrafts = make(map[string]string)
	}
	s.forgetComposerDraft(sessionID)
	s.composerDrafts[sessionID] = draft
	s.composerDraftOrder = append(s.composerDraftOrder, sessionID)
	for len(s.composerDraftOrder) > maxComposerDrafts {
		oldest := s.composerDraftOrder[0]
		s.composerDraftOrder = s.composerDraftOrder[1:]
		delete(s.composerDrafts, oldest)
	}
}

func (s *shell) takeComposerDraft(sessionID string) string {
	draft := s.composerDrafts[sessionID]
	s.forgetComposerDraft(sessionID)
	return draft
}

func (s *shell) forgetComposerDraft(sessionID string) {
	if sessionID == "" {
		return
	}
	delete(s.composerDrafts, sessionID)
	for index, id := range s.composerDraftOrder {
		if id == sessionID {
			s.composerDraftOrder = append(s.composerDraftOrder[:index], s.composerDraftOrder[index+1:]...)
			break
		}
	}
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

func activeQuestion(state desktopstate.State, sessionID string) *desktopstate.QuestionRequest {
	for _, request := range state.QuestionInbox {
		if request.SessionID == sessionID {
			item := request
			return &item
		}
	}
	return nil
}

// makeConversationCacheKey keys render caches by stable timeline identity rather
// than the sliding slice index. Retention trimming shifts indices but preserves
// item IDs, so an index-based key invalidated every cached markdown span and
// response split whenever the timeline overflowed.
func makeConversationCacheKey(sessionID string, index int, item desktopstate.TimelineItem) conversationCacheKey {
	itemID := item.ID
	if itemID == "" {
		itemID = "#" + fmt.Sprint(index)
	}
	return conversationCacheKey{
		sessionID: sessionID,
		itemID:    itemID,
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
