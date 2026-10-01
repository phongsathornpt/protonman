//go:build desktop || desktop_gio

package shell

import (
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Assistant thinking blocks: parsing the streamed reasoning channel out of the
// response text and rendering it behind a disclosure.
type parsedAssistantMessage struct {
	hasThinking  bool
	thinkingDone bool
	thinkingText string
	responseText string
}

func parseAssistantThinking(text string) parsedAssistantMessage {
	parsed := conversationcomponent.ParseAssistantThinking(text)
	return parsedAssistantMessage{
		hasThinking:  parsed.HasThinking,
		thinkingDone: parsed.ThinkingDone,
		thinkingText: parsed.ThinkingText,
		responseText: parsed.ResponseText,
	}
}

func (s *Shell) parsedThinking(key conversationCacheKey, text string) parsedAssistantMessage {
	if cached, ok := s.conversationThinkingCache.Get(key); ok && cached.source == text {
		return cached.parsed
	}
	parsed := parseAssistantThinking(text)
	s.conversationThinkingCache.Put(key, thinkingParseCacheEntry{source: text, parsed: parsed})
	return parsed
}

func (s *Shell) layoutThinkingBlock(gtx layout.Context, key conversationCacheKey, parsed parsedAssistantMessage) layout.Dimensions {
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

	chevron := uikit.KindChevronRight
	if expanded {
		chevron = uikit.KindChevronDown
	}

	headerBg := s.theme.Colors.SurfaceContainerLow
	headerBorder := s.theme.Colors.OutlineVariant
	if btn.Hovered() {
		headerBg = s.theme.Colors.SurfaceContainerHigh
		headerBorder = s.theme.Colors.Tertiary
	}

	return uikit.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedBorderSurface(gtx, shapeMedium, headerBg, headerBorder, 1, func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.LayoutActionIcon(gtx, uikit.KindStar, 14, s.theme.Colors.Tertiary)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, statusLabel, textLabelMedium, font.Medium, s.theme.Colors.Tertiary, 1)
									})
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{}.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.LayoutActionIcon(gtx, chevron, 12, s.theme.Colors.OnSurfaceVariant)
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
				return uikit.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedBorderSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLowest, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, thinkingText, textBodySmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 0)
						})
					})
				})
			}),
		)
	})
}

func (s *Shell) layoutActiveThinkingIndicator(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	label := "Protonman is working…"
	if session.Runtime.Reasoning != "" && session.Runtime.Reasoning != "none" {
		label = "Thinking and working (reasoning: " + session.Runtime.Reasoning + ")…"
	}

	return uikit.Inset{Top: 6, Bottom: 10, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeFull, s.theme.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.LayoutActionIcon(gtx, uikit.KindStar, 14, s.theme.Colors.OnPrimaryContainer)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, s.theme.Colors.OnPrimaryContainer, 1)
						})
					}),
				)
			})
		})
	})
}
