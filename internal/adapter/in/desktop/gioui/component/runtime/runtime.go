//go:build desktop || desktop_gio

// Package runtime owns Gio state for session runtime controls and popovers.
// The shell supplies snapshots and callbacks to the runtime views.
package runtime

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

// Chrome is the shared uikit surface. The runtime package adds no visual
// primitives of its own, so it uses the common palette directly.
type Chrome = uikit.Chrome

type Widgets struct {
	RuntimeProviderEditor, RuntimeModelEditor    widget.Editor
	RuntimeApplyButton                           widget.Clickable
	ReasoningButtons                             map[string]*widget.Clickable
	LowConcurrencyButtons                        map[string]*widget.Clickable
	PermissionModeButtons                        map[string]*widget.Clickable
	RuntimeEditorKey, RuntimeEditorProvider      string
	RuntimeEditorModel                           string
	ModelChipButton, ReasoningChipButton         widget.Clickable
	PermissionModeChipButton                     widget.Clickable
	ModelPopoverVisible, ReasoningPopoverVisible bool
	PermissionModePopoverVisible                 bool
	ModelSearchFocusPending                      bool
	ModelPopoverCloseButton                      widget.Clickable
	ReasoningPopoverCloseButton                  widget.Clickable
	PermissionModePopoverCloseButton             widget.Clickable
	ModelSearchEditor                            widget.Editor
	ModelSearchClearButton                       widget.Clickable
	ModelRefreshButton                           widget.Clickable
	PopoverProviderEditor, PopoverModelEditor    widget.Editor
	PopoverApplyModelButton                      widget.Clickable
	PopoverActiveProviderTab                     string
	PopoverProviderTabButtons                    map[string]*widget.Clickable
	PopoverProviderTabList                       layout.List
	PopoverManageProvidersButton                 widget.Clickable
	ModelList                                    layout.List
	AgentModelButtons                            map[string]*widget.Clickable
	PopoverReasoningButtons                      map[string]*widget.Clickable
	PopoverPermissionModeButtons                 map[string]*widget.Clickable
}

type Component struct {
	widgets Widgets
}

func New() *Component {
	return &Component{widgets: Widgets{
		RuntimeProviderEditor:        widget.Editor{SingleLine: true, MaxLen: 512},
		RuntimeModelEditor:           widget.Editor{SingleLine: true, MaxLen: 512},
		ModelSearchEditor:            widget.Editor{SingleLine: true, MaxLen: 256},
		PopoverProviderEditor:        widget.Editor{SingleLine: true, MaxLen: 256},
		PopoverModelEditor:           widget.Editor{SingleLine: true, MaxLen: 256},
		PopoverProviderTabButtons:    make(map[string]*widget.Clickable),
		AgentModelButtons:            make(map[string]*widget.Clickable),
		PopoverReasoningButtons:      make(map[string]*widget.Clickable),
		PopoverPermissionModeButtons: make(map[string]*widget.Clickable),
		ReasoningButtons:             make(map[string]*widget.Clickable),
		LowConcurrencyButtons:        make(map[string]*widget.Clickable),
		PermissionModeButtons:        make(map[string]*widget.Clickable),
		PopoverProviderTabList:       layout.List{Axis: layout.Horizontal},
		ModelList:                    layout.List{Axis: layout.Vertical},
	}}
}

func (c *Component) Widgets() *Widgets { return &c.widgets }

func (c *Component) ClosePopovers() {
	c.widgets.ModelPopoverVisible = false
	c.widgets.ReasoningPopoverVisible = false
	c.widgets.PermissionModePopoverVisible = false
}

func (c *Component) PopoverPermissionModeButton(mode string) *widget.Clickable {
	btn, ok := c.widgets.PopoverPermissionModeButtons[mode]
	if !ok {
		btn = new(widget.Clickable)
		if c.widgets.PopoverPermissionModeButtons == nil {
			c.widgets.PopoverPermissionModeButtons = make(map[string]*widget.Clickable)
		}
		c.widgets.PopoverPermissionModeButtons[mode] = btn
	}
	return btn
}

func (c *Component) AgentModelButton(name string) *widget.Clickable {
	btn, ok := c.widgets.AgentModelButtons[name]
	if !ok {
		btn = new(widget.Clickable)
		if c.widgets.AgentModelButtons == nil {
			c.widgets.AgentModelButtons = make(map[string]*widget.Clickable)
		}
		c.widgets.AgentModelButtons[name] = btn
	}
	return btn
}

func (c *Component) PopoverReasoningButton(level string) *widget.Clickable {
	btn, ok := c.widgets.PopoverReasoningButtons[level]
	if !ok {
		btn = new(widget.Clickable)
		if c.widgets.PopoverReasoningButtons == nil {
			c.widgets.PopoverReasoningButtons = make(map[string]*widget.Clickable)
		}
		c.widgets.PopoverReasoningButtons[level] = btn
	}
	return btn
}

func (c *Component) PopoverProviderTabButton(id string) *widget.Clickable {
	btn, ok := c.widgets.PopoverProviderTabButtons[id]
	if !ok {
		btn = new(widget.Clickable)
		if c.widgets.PopoverProviderTabButtons == nil {
			c.widgets.PopoverProviderTabButtons = make(map[string]*widget.Clickable)
		}
		c.widgets.PopoverProviderTabButtons[id] = btn
	}
	return btn
}
