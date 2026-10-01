//go:build desktop || desktop_gio

package shell

import (
	"strings"

	"gioui.org/layout"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
)

func (s *Shell) updateMentionState(workDir string) {
	s.updateMentionStateForText(workDir, s.conversationUI.Editor().Text())
}

func (s *Shell) updateMentionStateForText(workDir, text string) {
	runes := []rune(text)
	_, end := s.conversationUI.Editor().Selection()
	if end < 0 {
		end = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	textBeforeCaret := string(runes[:end])
	ctx, ok := conversationcomponent.ParseMentionContext(textBeforeCaret)
	if !ok {
		wasMention := s.mentionActive
		s.mentionActive = false
		s.mentionDismissed = false
		s.mentionDismissedQuery = ""
		s.mentionItems = nil
		s.mentionContext = conversationcomponent.MentionContext{}
		if wasMention && s.tailFollowBeforeOverlay && !s.runtimeComponent.Widgets().ModelPopoverVisible && !s.runtimeComponent.Widgets().ReasoningPopoverVisible {
			s.conversationUI.Timeline().ScrollToEnd = true
			s.conversationUI.Timeline().Position = layout.Position{}
			s.tailFollowBeforeOverlay = false
		}
		return
	}
	if s.mentionDismissed && ctx.Query == s.mentionDismissedQuery {
		s.mentionActive = false
		return
	}
	s.mentionDismissed = false
	wasActive := s.mentionActive
	previousItems := s.mentionItems
	if !wasActive && !s.conversationUI.Timeline().Position.BeforeEnd {
		s.tailFollowBeforeOverlay = true
	}
	s.mentionActive = true
	s.mentionContext = ctx
	s.closePopovers()
	agents := conversationcomponent.DefaultMentionAgents()
	workspaceFiles := s.mentionCache.get(workDir)
	if workspaceFiles == nil && strings.TrimSpace(workDir) != "" && wasActive && len(previousItems) > 0 {
		s.mentionItems = conversationcomponent.MatchMentionItems(ctx, agents, nil)
		if len(s.mentionItems) == 0 {
			s.mentionItems = previousItems
		}
	} else {
		s.mentionItems = conversationcomponent.MatchMentionItems(ctx, agents, workspaceFiles)
	}
	if len(s.mentionItems) == 0 {
		s.mentionSelectedIndex = 0
	} else if s.mentionSelectedIndex >= len(s.mentionItems) {
		s.mentionSelectedIndex = len(s.mentionItems) - 1
	} else if s.mentionSelectedIndex < 0 {
		s.mentionSelectedIndex = 0
	}
}

func (s *Shell) refreshMentionState(workDir string) {
	if !s.mentionStateDirty && s.mentionWorkspace == workDir {
		return
	}
	text := s.conversationUI.Editor().Text()
	s.composerHasContent = strings.TrimSpace(text) != ""
	s.updateMentionStateForText(workDir, text)
	s.mentionStateDirty = false
	s.mentionWorkspace = workDir
}

func (s *Shell) setComposerText(text string) {
	s.conversationUI.Editor().SetText(text)
	s.mentionStateDirty = true
	s.composerHasContent = strings.TrimSpace(text) != ""
}

func (s *Shell) applySelectedMention() {
	if !s.mentionActive || len(s.mentionItems) == 0 {
		return
	}
	if s.mentionSelectedIndex < 0 || s.mentionSelectedIndex >= len(s.mentionItems) {
		s.mentionSelectedIndex = 0
	}
	item := s.mentionItems[s.mentionSelectedIndex]
	s.insertMention(item)
}

func (s *Shell) insertMention(item conversationcomponent.MentionItem) {
	text := s.conversationUI.Editor().Text()
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
	s.conversationUI.Editor().SetCaret(newCaret, newCaret)
	s.mentionActive = false
	s.mentionDismissed = false
	s.mentionDismissedQuery = ""
	s.mentionItems = nil
	s.mentionContext = conversationcomponent.MentionContext{}
	if s.tailFollowBeforeOverlay && !s.runtimeComponent.Widgets().ModelPopoverVisible && !s.runtimeComponent.Widgets().ReasoningPopoverVisible {
		s.conversationUI.Timeline().ScrollToEnd = true
		s.conversationUI.Timeline().Position = layout.Position{}
		s.tailFollowBeforeOverlay = false
	}
}

func (s *Shell) layoutMentionPopup(gtx layout.Context) layout.Dimensions {
	if !s.mentionActive {
		return layout.Dimensions{}
	}
	return s.conversationUI.LayoutMentions(gtx, s.mentionItems, s.mentionSelectedIndex, s.mentionChrome(), s.insertMention)
}

func (s *Shell) mentionChrome() conversationcomponent.MentionChrome {
	return conversationcomponent.MentionChrome{
		OnSurface: s.theme.Colors.OnSurface, OnSurfaceVariant: s.theme.Colors.OnSurfaceVariant,
		OnPrimaryContainer: s.theme.Colors.OnPrimaryContainer, OnSecondaryContainer: s.theme.Colors.OnSecondaryContainer,
		PrimaryContainer: s.theme.Colors.PrimaryContainer, SecondaryContainer: s.theme.Colors.SecondaryContainer,
		SurfaceContainer: s.theme.Colors.SurfaceContainer, SurfaceContainerHigh: s.theme.Colors.SurfaceContainerHigh,
		SurfaceContainerHighest: s.theme.Colors.SurfaceContainerHighest, OutlineVariant: s.theme.Colors.OutlineVariant,
		Label: s.layoutLabel, RoundedSurface: s.roundedSurface, BorderSurface: s.roundedBorderSurface,
	}
}
