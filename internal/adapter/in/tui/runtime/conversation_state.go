package runtime

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/conversation"
)

func (s *conversationModelState) retainMessages() {
	if s == nil {
		return
	}
	s.messages = conversation.Retain(s.messages, s.conversationRetention)
}

func (s *conversationModelState) appendMessages(messages ...model.Message) {
	if s == nil || len(messages) == 0 {
		return
	}
	s.messages = append(s.messages, messages...)
	s.retainMessages()
}

func (s *conversationModelState) dropTrailingUserMessage() bool {
	if s == nil || len(s.messages) == 0 || s.messages[len(s.messages)-1].Role != model.RoleUser {
		return false
	}
	last := len(s.messages) - 1
	s.messages[last] = model.Message{}
	s.messages = s.messages[:last]
	return true
}
func (s *conversationModelState) enqueue(line string) bool {
	if s == nil || len(s.queue) >= maxQueuedPrompts {
		return false
	}
	s.queue = append(s.queue, line)
	return true
}

func (s *conversationModelState) dequeue() (string, bool) {
	if s == nil || len(s.queue) == 0 {
		return "", false
	}
	line := s.queue[0]
	s.queue[0] = ""
	if len(s.queue) == 1 {
		s.queue = nil
	} else {
		s.queue = s.queue[1:]
	}
	return line, true
}

func (s *conversationModelState) clearQueue() {
	if s != nil {
		clear(s.queue)
		s.queue = nil
	}
}

func (s *conversationModelState) resetConversationData() {
	if s == nil {
		return
	}
	clear(s.messages)
	s.messages = nil
	s.clearQueue()
	s.conversationViewport = conversationViewportState{mode: viewportFollowing}
}
