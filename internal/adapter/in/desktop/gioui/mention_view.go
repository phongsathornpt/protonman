//go:build desktop || desktop_gio

package gioui

import (
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"
)

func (s *shell) updateMentionState(workDir string) {
	s.updateMentionStateForText(workDir, s.composer.Text())
}

func (s *shell) updateMentionStateForText(workDir, text string) {
	runes := []rune(text)
	_, end := s.composer.Selection()
	if end < 0 {
		end = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	textBeforeCaret := string(runes[:end])
	ctx, ok := parseMentionContext(textBeforeCaret)
	if !ok {
		s.mentionActive = false
		s.mentionDismissed = false
		s.mentionDismissedQuery = ""
		s.mentionItems = nil
		s.mentionContext = MentionContext{}
		return
	}
	if s.mentionDismissed && ctx.Query == s.mentionDismissedQuery {
		s.mentionActive = false
		return
	}
	s.mentionDismissed = false
	wasActive := s.mentionActive
	previousItems := s.mentionItems
	s.mentionActive = true
	s.mentionContext = ctx
	s.closePopovers()
	agents := defaultMentionAgents()
	workspaceFiles := s.mentionCache.get(workDir)
	if workspaceFiles == nil && strings.TrimSpace(workDir) != "" && wasActive && len(previousItems) > 0 {
		s.mentionItems = matchMentionItems(ctx, agents, nil)
		if len(s.mentionItems) == 0 {
			s.mentionItems = previousItems
		}
	} else {
		s.mentionItems = matchMentionItems(ctx, agents, workspaceFiles)
	}
	if len(s.mentionItems) == 0 {
		s.mentionSelectedIndex = 0
	} else if s.mentionSelectedIndex >= len(s.mentionItems) {
		s.mentionSelectedIndex = len(s.mentionItems) - 1
	} else if s.mentionSelectedIndex < 0 {
		s.mentionSelectedIndex = 0
	}
}

func (s *shell) refreshMentionState(workDir string) {
	if !s.mentionStateDirty && s.mentionWorkspace == workDir {
		return
	}
	text := s.composer.Text()
	s.composerHasContent = strings.TrimSpace(text) != ""
	s.updateMentionStateForText(workDir, text)
	s.mentionStateDirty = false
	s.mentionWorkspace = workDir
}

func (s *shell) setComposerText(text string) {
	s.composer.SetText(text)
	s.mentionStateDirty = true
	s.composerHasContent = strings.TrimSpace(text) != ""
}

func (s *shell) applySelectedMention() {
	if !s.mentionActive || len(s.mentionItems) == 0 {
		return
	}
	if s.mentionSelectedIndex < 0 || s.mentionSelectedIndex >= len(s.mentionItems) {
		s.mentionSelectedIndex = 0
	}
	item := s.mentionItems[s.mentionSelectedIndex]
	s.insertMention(item)
}

func (s *shell) insertMention(item MentionItem) {
	text := s.composer.Text()
	runes := []rune(text)
	start := s.mentionContext.StartOffset
	end := s.mentionContext.EndOffset
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start > end {
		start = end
	}
	insertion := item.InsertionText()
	newRunes := append(runes[:start], append([]rune(insertion), runes[end:]...)...)
	s.setComposerText(string(newRunes))
	newCaret := start + len([]rune(insertion))
	s.composer.SetCaret(newCaret, newCaret)
	s.mentionActive = false
	s.mentionDismissed = false
	s.mentionDismissedQuery = ""
	s.mentionItems = nil
	s.mentionContext = MentionContext{}
}

func (s *shell) layoutMentionPopup(gtx layout.Context) layout.Dimensions {
	if !s.mentionActive || len(s.mentionItems) == 0 {
		return layout.Dimensions{}
	}
	maxHeight := gtx.Dp(220)
	gtx.Constraints.Max.Y = maxHeight
	return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 4, Bottom: 4, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				s.mentionList.Axis = layout.Vertical
				return s.mentionList.Layout(gtx, len(s.mentionItems), func(gtx layout.Context, index int) layout.Dimensions {
					item := s.mentionItems[index]
					selected := index == s.mentionSelectedIndex
					return s.layoutMentionItem(gtx, item, selected)
				})
			})
		})
	})
}

func (s *shell) layoutMentionItem(gtx layout.Context, item MentionItem, selected bool) layout.Dimensions {
	btn, ok := s.mentionButtons[item.Name]
	if !ok {
		btn = new(widget.Clickable)
		if s.mentionButtons == nil {
			s.mentionButtons = make(map[string]*widget.Clickable)
		}
		s.mentionButtons[item.Name] = btn
	}
	if btn.Clicked(gtx) {
		s.insertMention(item)
	}
	bgColor := s.theme.surfaceContainerHigh
	if selected || btn.Hovered() {
		bgColor = s.theme.primaryContainer
	}
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeSmall, bgColor, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						tagBg := s.theme.surfaceContainer
						tagFg := s.theme.onSurfaceVariant
						switch item.Kind {
						case MentionItemKindAgent:
							tagBg = s.theme.primaryContainer
							tagFg = s.theme.onPrimaryContainer
						case MentionItemKindFile:
							tagBg = s.theme.secondaryContainer
							tagFg = s.theme.onSecondaryContainer
						case MentionItemKindDir:
							tagBg = s.theme.surfaceContainerHighest
							tagFg = s.theme.onSurfaceVariant
						}
						return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedSurface(gtx, shapeSmall, tagBg, func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, item.PrefixTag, textLabelSmall, font.Medium, tagFg, 1)
								})
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						displayName := item.Name
						if item.Kind == MentionItemKindDir && !strings.HasSuffix(displayName, "/") {
							displayName += "/"
						}
						fg := s.theme.onSurface
						if selected {
							fg = s.theme.onPrimaryContainer
						}
						return s.layoutLabel(gtx, "@"+displayName, textBodyMedium, font.Medium, fg, 1)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if item.Description == "" {
							return layout.Spacer{}.Layout(gtx)
						}
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							fg := s.theme.onSurfaceVariant
							return s.layoutLabel(gtx, item.Description, textLabelSmall, font.Normal, fg, 1)
						})
					}),
				)
			})
		})
	})
}
