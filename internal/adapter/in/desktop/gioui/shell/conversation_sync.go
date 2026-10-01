//go:build desktop || desktop_gio

package shell

import (
	"strings"

	"gioui.org/layout"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Synchronizes controller snapshot state into the render caches and widget sets
// before a frame is laid out.
func (s *Shell) syncConversation(state desktopstate.State) {
	if state.ActiveSessionID == s.activeSessionID && state.ActiveAgentID == s.activeSessionAgentID {
		s.syncPermissionButtons(state)
		return
	}
	if s.activeSessionID != "" {
		previousRef := desktopstate.SessionRef{AgentID: s.activeSessionAgentID, SessionID: s.activeSessionID}
		s.sessionScrollPositions.Put(previousRef, s.conversationUI.Timeline().Position)
		if draft := s.conversationUI.Editor().Text(); strings.TrimSpace(draft) != "" {
			s.rememberComposerDraft(controller.SessionRefStorageKey(previousRef), draft)
		} else {
			s.forgetComposerDraft(controller.SessionRefStorageKey(previousRef))
		}
	}
	s.activeSessionID = state.ActiveSessionID
	s.activeSessionAgentID = state.ActiveAgentID
	if state.ActiveSessionID != "" {
		rows, _ := buildSidebarRows(state, nil)
		for index, row := range rows {
			if row.Kind == sidebarSessionRow && row.SessionID == state.ActiveSessionID && (state.ActiveAgentID == "" || row.AgentID == state.ActiveAgentID) {
				s.sidebarData.ScrollTo(max(0, index-1))
				break
			}
		}
	}
	activeRef := desktopstate.SessionRef{AgentID: state.ActiveAgentID, SessionID: state.ActiveSessionID}
	s.setComposerText(s.takeComposerDraft(controller.SessionRefStorageKey(activeRef)))
	s.closePopovers()
	s.tailFollowBeforeOverlay = false
	if position, ok := s.sessionScrollPositions.Get(activeRef); ok {
		s.conversationUI.Timeline().Position = position
	} else {
		s.conversationUI.Timeline().Position = layout.Position{}
	}
	s.inspectorComponent.ResetVisibility()
	s.runtimeComponent.Widgets().RuntimeEditorKey = ""
	clear(s.conversationResponseCache)
	s.conversationResponseBytes = 0
	s.conversationResponseOrder = s.conversationResponseOrder[:0]
	clear(s.conversationExpanded)
	clear(s.conversationPage)
	clear(s.conversationExpandButtons)
	clear(s.toolOutputPages)
	s.expansionOrder.Reset()
	clear(s.conversationThinkingExpanded)
	clear(s.conversationThinkingButtons)
	s.conversationThinkingCache.Reset()
	s.toolDiffCache.Reset()
	clear(s.permissionButtons)
	clear(s.toolExpanded)
	clear(s.toolExpandButtons)
	clear(s.messageCopyButtons)
	clear(s.messageCopiedAt)
	clear(s.codeCopyButtons)
	clear(s.codeCopiedAt)
	clear(s.userRetryButtons)
	s.conversationUI.ResetMentionButtons()
	clear(s.runtimeComponent.Widgets().AgentModelButtons)
	clear(s.runtimeComponent.Widgets().ModelPresetButtons)
	clear(s.runtimeComponent.Widgets().PopoverReasoningButtons)
	clear(s.runtimeComponent.Widgets().PopoverPermissionModeButtons)
	clear(s.questionStates)
	s.permissionButtonRevision = 0
	s.permissionButtonRevisionSet = false
	s.mentionStateDirty = true
}

func (s *Shell) syncPermissionButtons(state desktopstate.State) {
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

func (s *Shell) conversationDescription(key conversationCacheKey, item desktopstate.TimelineItem) string {
	if cached, ok := s.conversationDescriptions.Get(key); ok && cached.source == item {
		return cached.description
	}
	description := conversationItemDescription(item)
	s.conversationDescriptions.Put(key, descriptionCacheEntry{source: item, description: description})
	return description
}
