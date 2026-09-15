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

// Attachment is a TUI-local attachment reference. The path is retained until
// the submission is snapshotted into a provider-neutral model message.
type Attachment struct {
	Placeholder string
	Path        string
	// Temporary means the TUI owns Path and may delete it once the attachment
	// has been snapshotted, discarded, or removed from the queue.
	Temporary bool
}

// QueuedInput preserves the complete user submission while another turn or
// image preparation step owns execution.
type QueuedInput struct {
	Text        string
	Attachments []Attachment
}

func (q QueuedInput) Clone() QueuedInput {
	q.Attachments = append([]Attachment(nil), q.Attachments...)
	return q
}

// State encapsulates conversation message history and queued prompts for the TUI runtime.
type State struct {
	messages         []model.Message
	queue            []QueuedInput
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

// Enqueue preserves the historical text-only API for command and test callers.
func (s *State) Enqueue(line string) bool {
	return s.EnqueueInput(QueuedInput{Text: line})
}

// EnqueueInput adds one complete user submission to the queue if capacity permits.
func (s *State) EnqueueInput(input QueuedInput) bool {
	limit := DefaultMaxQueuedPrompts
	if s != nil && s.maxQueuedPrompts > 0 {
		limit = s.maxQueuedPrompts
	}
	if s == nil || len(s.queue) >= limit {
		return false
	}
	s.queue = append(s.queue, input.Clone())
	return true
}

// Dequeue preserves the historical text-only API.
func (s *State) Dequeue() (string, bool) {
	input, ok := s.DequeueInput()
	if !ok {
		return "", false
	}
	return input.Text, true
}

// DequeueInput removes and returns the first complete queued submission.
func (s *State) DequeueInput() (QueuedInput, bool) {
	if s == nil || len(s.queue) == 0 {
		return QueuedInput{}, false
	}
	input := s.queue[0].Clone()
	s.queue[0] = QueuedInput{}
	if len(s.queue) == 1 {
		s.queue = nil
	} else {
		s.queue = s.queue[1:]
	}
	return input, true
}

// ClearQueue clears all queued prompts.
func (s *State) ClearQueue() {
	if s != nil {
		clear(s.queue)
		s.queue = nil
	}
}

// Queue returns queued prompt text for legacy display/test callers.
func (s *State) Queue() []string {
	if s == nil || len(s.queue) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.queue))
	for _, input := range s.queue {
		out = append(out, input.Text)
	}
	return out
}

// QueuedInputs returns a defensive copy of complete queued submissions.
func (s *State) QueuedInputs() []QueuedInput {
	if s == nil || len(s.queue) == 0 {
		return nil
	}
	out := make([]QueuedInput, 0, len(s.queue))
	for _, input := range s.queue {
		out = append(out, input.Clone())
	}
	return out
}

// QueueLen returns the current queued prompt count.
func (s *State) QueueLen() int {
	if s == nil {
		return 0
	}
	return len(s.queue)
}
