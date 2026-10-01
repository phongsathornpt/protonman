//go:build desktop || desktop_gio

package conversation

import (
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

type ParsedAssistantMessage struct {
	HasThinking  bool
	ThinkingDone bool
	ThinkingText string
	ResponseText string
}

func ParseAssistantThinking(text string) ParsedAssistantMessage {
	const openTag = "<think>"
	const closeTag = "</think>"

	openIdx := strings.Index(text, openTag)
	if openIdx == -1 {
		return ParsedAssistantMessage{
			HasThinking:  false,
			ResponseText: text,
		}
	}

	beforeThink := text[:openIdx]
	afterOpen := text[openIdx+len(openTag):]

	closeIdx := strings.Index(afterOpen, closeTag)
	if closeIdx == -1 {
		return ParsedAssistantMessage{
			HasThinking:  true,
			ThinkingDone: false,
			ThinkingText: strings.TrimSpace(afterOpen),
			ResponseText: strings.TrimSpace(beforeThink),
		}
	}

	thinking := afterOpen[:closeIdx]
	afterClose := afterOpen[closeIdx+len(closeTag):]

	resp := strings.TrimSpace(beforeThink)
	trimmedAfter := strings.TrimSpace(afterClose)
	if resp != "" && trimmedAfter != "" {
		resp += "\n\n" + trimmedAfter
	} else if resp == "" {
		resp = trimmedAfter
	}

	return ParsedAssistantMessage{
		HasThinking:  true,
		ThinkingDone: true,
		ThinkingText: strings.TrimSpace(thinking),
		ResponseText: resp,
	}
}

type ThinkingBlockInput struct {
	Button      *widget.Clickable
	Expanded    bool
	HasExplicit bool
	Parsed      ParsedAssistantMessage
	StreamingFn func(string) string
	Chrome      Chrome
	OnToggle    func()
}

func LayoutThinkingBlock(gtx layout.Context, input ThinkingBlockInput) layout.Dimensions {
	chrome := input.Chrome
	btn := input.Button
	if btn == nil {
		btn = new(widget.Clickable)
	}
	if btn.Clicked(gtx) && input.OnToggle != nil {
		input.OnToggle()
	}

	expanded := input.Expanded
	if !input.HasExplicit {
		expanded = !input.Parsed.ThinkingDone
	}

	thinkingText := input.Parsed.ThinkingText
	if !input.Parsed.ThinkingDone && input.StreamingFn != nil {
		thinkingText = input.StreamingFn(input.Parsed.ThinkingText)
	}

	statusLabel := "Thought process"
	if !input.Parsed.ThinkingDone {
		statusLabel = "Thinking in progress…"
	} else if count := strings.Count(input.Parsed.ThinkingText, "\n") + 1; count > 1 {
		statusLabel = fmt.Sprintf("Thought process (%d lines)", count)
	}

	chevron := uikit.IconChevronRight
	if expanded {
		chevron = uikit.IconChevronDown
	}

	headerBg := chrome.Colors.SurfaceContainerLow
	headerBorder := chrome.Colors.OutlineVariant
	if btn.Hovered() {
		headerBg = chrome.Colors.SurfaceContainerHigh
		headerBorder = chrome.Colors.Tertiary
	}

	return chrome.Inset(gtx, layout.Inset{Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return chrome.BorderSurface(gtx, unit.Dp(8), headerBg, headerBorder, 1, func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.ActionIcon(gtx, "star", unit.Dp(14), chrome.Colors.Tertiary)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, statusLabel, unit.Sp(12), font.Medium, chrome.Colors.Tertiary, 1)
									})
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{}.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.ActionIcon(gtx, chevron, unit.Dp(12), chrome.Colors.OnSurfaceVariant)
								}),
							)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !expanded || strings.TrimSpace(input.Parsed.ThinkingText) == "" {
					return layout.Dimensions{}
				}
				return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
					return chrome.BorderSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLowest, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, thinkingText, unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 0)
						})
					})
				})
			}),
		)
	})
}

func LayoutActiveThinkingIndicator(gtx layout.Context, reasoning string, chrome Chrome) layout.Dimensions {
	label := "Protonman is working…"
	if reasoning != "" && reasoning != "none" {
		label = "Thinking and working (reasoning: " + reasoning + ")…"
	}

	return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 10, Left: 16, Right: 16}, func(gtx layout.Context) layout.Dimensions {
		return chrome.RoundedSurface(gtx, unit.Dp(9999), chrome.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.ActionIcon(gtx, "star", unit.Dp(14), chrome.Colors.OnPrimaryContainer)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, label, unit.Sp(12), font.Medium, chrome.Colors.OnPrimaryContainer, 1)
						})
					}),
				)
			})
		})
	})
}
