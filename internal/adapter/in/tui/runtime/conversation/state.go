package conversation

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	coreconv "github.com/phongsathornpt/protonman/internal/core/conversation"
)

const (
	// DefaultMaxQueuedPrompts is the maximum number of prompts allowed in the queue.
	DefaultMaxQueuedPrompts = 32
	// DefaultMaxQueuePreviewRunes is the maximum number of runes in a queued prompt preview.
	DefaultMaxQueuePreviewRunes = 160
)

// State encapsulates conversation message history and queued prompts for the TUI runtime.
type State struct {
	messages         []model.Message
	queue            []string
	retention        coreconv.RetentionPolicy
	maxQueuedPrompts int
}

// NewState creates a new conversation state with the given retention policy and optional initial messages.
func NewState(retention coreconv.RetentionPolicy, initialMessages ...[]model.Message) *State {
	var msgs []model.Message
	if len(initialMessages) > 0 && len(initialMessages[0]) > 0 {
		msgs = coreconv.Retain(model.SnapshotMessages(initialMessages[0]), retention)
	}
	return &State{
		messages:         msgs,
		queue:            make([]string, 0),
		retention:        retention,
		maxQueuedPrompts: DefaultMaxQueuedPrompts,
	}
}

// SetMaxQueuedPrompts overrides the default queue capacity limit.
func (s *State) SetMaxQueuedPrompts(limit int) {
	if s == nil || limit <= 0 {
		return
	}
	s.maxQueuedPrompts = limit
}

// Messages returns the current slice of conversation messages.
func (s *State) Messages() []model.Message {
	if s == nil {
		return nil
	}
	return s.messages
}

// SetMessages sets the conversation messages slice directly.
func (s *State) SetMessages(messages []model.Message) {
	if s == nil {
		return
	}
	s.messages = messages
}

// SnapshotMessages returns a deep copy of current conversation messages.
func (s *State) SnapshotMessages() []model.Message {
	if s == nil {
		return nil
	}
	return model.SnapshotMessages(s.messages)
}

// Retention returns the active retention policy.
func (s *State) Retention() coreconv.RetentionPolicy {
	if s == nil {
		return coreconv.DefaultRetentionPolicy()
	}
	return s.retention
}

// SetRetention sets the active retention policy and applies it.
func (s *State) SetRetention(policy coreconv.RetentionPolicy) {
	if s == nil {
		return
	}
	s.retention = policy
	s.RetainMessages()
}

// RetainMessages applies the retention policy to truncate old messages.
func (s *State) RetainMessages() {
	if s == nil {
		return
	}
	s.messages = coreconv.Retain(s.messages, s.retention)
}

// AppendMessages appends messages and retains according to policy.
func (s *State) AppendMessages(messages ...model.Message) {
	if s == nil || len(messages) == 0 {
		return
	}
	s.messages = append(s.messages, messages...)
	s.RetainMessages()
}

// DropTrailingUserMessage drops the last message if its role is RoleUser.
// Returns true if a message was dropped, false otherwise.
func (s *State) DropTrailingUserMessage() bool {
	if s == nil || len(s.messages) == 0 || s.messages[len(s.messages)-1].Role != model.RoleUser {
		return false
	}
	last := len(s.messages) - 1
	s.messages[last] = model.Message{}
	s.messages = s.messages[:last]
	return true
}

// Enqueue adds a prompt line to the queue if capacity permits.
func (s *State) Enqueue(line string) bool {
	limit := DefaultMaxQueuedPrompts
	if s != nil && s.maxQueuedPrompts > 0 {
		limit = s.maxQueuedPrompts
	}
	if s == nil || len(s.queue) >= limit {
		return false
	}
	s.queue = append(s.queue, line)
	return true
}

// Dequeue removes and returns the first prompt from the queue.
func (s *State) Dequeue() (string, bool) {
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

// ClearQueue clears all queued prompts.
func (s *State) ClearQueue() {
	if s != nil {
		clear(s.queue)
		s.queue = nil
	}
}

// Queue returns the current queued prompt strings.
func (s *State) Queue() []string {
	if s == nil {
		return nil
	}
	return s.queue
}

// QueueLen returns the current queue length.
func (s *State) QueueLen() int {
	if s == nil {
		return 0
	}
	return len(s.queue)
}

// Reset clears messages and the queue while preserving retention policy.
func (s *State) Reset() {
	if s == nil {
		return
	}
	clear(s.messages)
	s.messages = nil
	s.ClearQueue()
}

// QueuePreview truncates a prompt string to a maximum number of runes for display.
func QueuePreview(line string, maxRunes ...int) string {
	limit := DefaultMaxQueuePreviewRunes
	if len(maxRunes) > 0 && maxRunes[0] > 0 {
		limit = maxRunes[0]
	}
	trimmed := strings.TrimSpace(line)
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "…"
}
