//go:build desktop || desktop_gio

// Package conversation owns the Gio widget and draft state for the transcript
// and composer surface. The shell supplies session snapshots and app actions.
package conversation

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
)

type Component struct {
	timeline           layout.List
	composer           widget.Editor
	mentionList        layout.List
	mentionButtons     map[string]*widget.Clickable
	composerDrafts     map[string]string
	composerDraftOrder []string
	maxDrafts          int
	send               widget.Clickable
	stop               widget.Clickable
	jumpToBottom       widget.Clickable
	starters           [4]widget.Clickable
}

func New(maxDrafts int) *Component {
	if maxDrafts < 1 {
		maxDrafts = 1
	}
	return &Component{
		timeline:           layout.List{Axis: layout.Vertical, ScrollToEnd: true},
		composer:           widget.Editor{Submit: true, MaxLen: 1 << 20},
		mentionList:        layout.List{Axis: layout.Vertical},
		mentionButtons:     make(map[string]*widget.Clickable),
		composerDrafts:     make(map[string]string),
		composerDraftOrder: make([]string, 0, maxDrafts),
		maxDrafts:          maxDrafts,
	}
}

func (c *Component) Timeline() *layout.List { return &c.timeline }

func (c *Component) Editor() *widget.Editor { return &c.composer }

func (c *Component) MentionList() *layout.List { return &c.mentionList }

func (c *Component) MentionButton(key string) *widget.Clickable {
	if c.mentionButtons == nil {
		c.mentionButtons = make(map[string]*widget.Clickable)
	}
	button := c.mentionButtons[key]
	if button == nil {
		button = new(widget.Clickable)
		c.mentionButtons[key] = button
	}
	return button
}

func (c *Component) ResetMentionButtons() { clear(c.mentionButtons) }

func (c *Component) SendButton() *widget.Clickable { return &c.send }

func (c *Component) StopButton() *widget.Clickable { return &c.stop }

func (c *Component) JumpToBottomButton() *widget.Clickable { return &c.jumpToBottom }

func (c *Component) StarterButton(index int) *widget.Clickable {
	if index < 0 || index >= len(c.starters) {
		return nil
	}
	return &c.starters[index]
}

func (c *Component) SaveDraft(key, draft string) {
	if key == "" || strings.TrimSpace(draft) == "" {
		return
	}
	if c.composerDrafts == nil {
		c.composerDrafts = make(map[string]string)
	}
	c.ForgetDraft(key)
	c.composerDrafts[key] = draft
	c.composerDraftOrder = append(c.composerDraftOrder, key)
	for len(c.composerDraftOrder) > c.maxDrafts {
		oldest := c.composerDraftOrder[0]
		c.composerDraftOrder[0] = ""
		c.composerDraftOrder = c.composerDraftOrder[1:]
		delete(c.composerDrafts, oldest)
	}
}

func (c *Component) DraftCounts() (int, int) {
	return len(c.composerDrafts), len(c.composerDraftOrder)
}

func (c *Component) TakeDraft(key string) string {
	draft := c.composerDrafts[key]
	c.ForgetDraft(key)
	return draft
}

func (c *Component) ForgetDraft(key string) {
	if c == nil || key == "" {
		return
	}
	delete(c.composerDrafts, key)
	for index, existing := range c.composerDraftOrder {
		if existing == key {
			copy(c.composerDraftOrder[index:], c.composerDraftOrder[index+1:])
			c.composerDraftOrder[len(c.composerDraftOrder)-1] = ""
			c.composerDraftOrder = c.composerDraftOrder[:len(c.composerDraftOrder)-1]
			return
		}
	}
}
