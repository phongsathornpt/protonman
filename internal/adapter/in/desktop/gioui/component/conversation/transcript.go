//go:build desktop || desktop_gio

package conversation

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type TranscriptInput struct {
	Session        desktopstate.SessionState
	HistoryLoading bool
	OverlayOpen    func() bool
	MaxTextWidth   unit.Dp
	Chrome         TranscriptChrome
	Empty          func(layout.Context) layout.Dimensions
	TimelineItem   func(layout.Context, int, desktopstate.TimelineItem) layout.Dimensions
	SubagentItem   func(layout.Context, desktopstate.SubagentState) layout.Dimensions
	ActiveThinking func(layout.Context, desktopstate.SessionState) layout.Dimensions
	JumpToBottom   func(layout.Context) layout.Dimensions
}

type TranscriptChrome struct {
	SecondaryContainer, OnSecondaryContainer color.NRGBA
	Label                                    func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions
	RoundedSurface                           func(layout.Context, unit.Dp, color.NRGBA, layout.Widget) layout.Dimensions
}

// LayoutTranscript owns the transcript viewport, scroll-follow policy, history
// banner placement, and jump-to-bottom slot. Content rows remain shell callbacks
// until their renderer is fully moved into this feature package.
func (c *Component) LayoutTranscript(gtx layout.Context, input TranscriptInput) layout.Dimensions {
	session := input.Session
	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = 0
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(input.MaxTextWidth))
		children := make([]layout.FlexChild, 0, 3)
		if input.HistoryLoading || session.HistoryTruncated {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 8, Bottom: 8, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return c.LayoutHistoryBanner(gtx, session.HistoryTruncated, input.Chrome)
				})
			}))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(session.Timeline) == 0 && len(session.Subagents) == 0 {
				if input.Empty == nil {
					return layout.Dimensions{}
				}
				return input.Empty(gtx)
			}
			runningActive := session.Status == desktopstate.TaskRunning
			itemCount := len(session.Timeline) + len(session.Subagents)
			if runningActive {
				itemCount++
			}
			itemCount++ // clearance at the end of the transcript

			dims := c.timeline.Layout(gtx, itemCount, func(gtx layout.Context, index int) layout.Dimensions {
				if index < len(session.Timeline) && input.TimelineItem != nil {
					return input.TimelineItem(gtx, index, session.Timeline[index])
				}
				subagentIndex := index - len(session.Timeline)
				if subagentIndex < len(session.Subagents) && subagentIndex >= 0 && input.SubagentItem != nil {
					return input.SubagentItem(gtx, session.Subagents[subagentIndex])
				}
				if runningActive && index == len(session.Timeline)+len(session.Subagents) && input.ActiveThinking != nil {
					return input.ActiveThinking(gtx, session)
				}
				return layout.Spacer{Height: 48}.Layout(gtx)
			})
			overlayOpen := input.OverlayOpen != nil && input.OverlayOpen()
			if c.timeline.Position.BeforeEnd || overlayOpen {
				c.timeline.ScrollToEnd = false
			} else {
				c.timeline.ScrollToEnd = true
			}
			return dims
		}))
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !c.timeline.Position.BeforeEnd || len(session.Timeline) <= 2 || input.JumpToBottom == nil {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: 4, Bottom: 8, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(40)
				gtx.Constraints.Max.Y = gtx.Dp(40)
				return layout.Stack{Alignment: layout.E}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
					layout.Stacked(input.JumpToBottom),
				)
			})
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}))
}

func (c *Component) LayoutHistoryBanner(gtx layout.Context, truncated bool, chrome TranscriptChrome) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(40)
	return chrome.RoundedSurface(gtx, unit.Dp(8), chrome.SecondaryContainer, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 10, Bottom: 10, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			message := "Loading session history…"
			if truncated {
				message = "Showing recent history; older entries were trimmed to keep the session responsive."
			}
			return chrome.Label(gtx, message, unit.Sp(14), font.Medium, chrome.OnSecondaryContainer, 2)
		})
	})
}
