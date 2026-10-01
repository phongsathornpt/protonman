//go:build desktop || desktop_gio

package shell

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Tool-call rendering: bounded streaming text, tool-output paging, diff
// detection, and the subagent activity item.
func streamingText(source string) string {
	return conversationcomponent.StreamingText(source)
}

func isDiffText(text string) bool {
	return conversationcomponent.IsDiffText(text)
}

func extractDiffFilename(text string) string {
	return conversationcomponent.ExtractDiffFilename(text)
}

func toolCategoryIcon(title string) uikit.IconKind {
	switch conversationcomponent.ToolCategoryIcon(title) {
	case "terminal":
		return uikit.KindTerminal
	case "folder":
		return uikit.KindFolder
	case "search":
		return uikit.KindSearch
	case "compose":
		return uikit.KindCompose
	default:
		return uikit.KindSettings
	}
}

func (s *Shell) toolDiffInfo(toolKey, text string) toolDiffCacheEntry {
	if cached, ok := s.toolDiffCache.Get(toolKey); ok && cached.source == text {
		return cached
	}
	res := conversationcomponent.ToolDiffInfo(nil, toolKey, text)
	entry := toolDiffCacheEntry{
		source:    res.Source,
		isDiff:    res.IsDiff,
		filename:  res.Filename,
		additions: res.Additions,
		deletions: res.Deletions,
		preview:   res.Preview,
		omitted:   res.Omitted,
	}
	s.toolDiffCache.Put(toolKey, entry)
	return entry
}

func (s *Shell) layoutToolItem(gtx layout.Context, sessionID string, index int, item desktopstate.TimelineItem, foreground color.NRGBA) layout.Dimensions {
	toolKey := item.ID
	if toolKey == "" {
		toolKey = fmt.Sprintf("%s:tool:%d", sessionID, index)
	}
	cacheKey := makeConversationCacheKey(sessionID, index, item)

	if s.toolExpandButtons == nil {
		s.toolExpandButtons = make(map[string]*widget.Clickable)
	}
	btn, ok := s.toolExpandButtons[toolKey]
	if !ok {
		btn = new(widget.Clickable)
		s.toolExpandButtons[toolKey] = btn
	}

	if s.toolExpanded == nil {
		s.toolExpanded = make(map[string]bool)
	}
	if btn.Clicked(gtx) {
		s.toolExpanded[toolKey] = !s.toolExpanded[toolKey]
	}

	status := strings.TrimSpace(item.Status)
	statusBg := s.theme.Colors.SecondaryContainer
	statusFg := s.theme.Colors.OnSecondaryContainer
	statusLabel := "TOOL"
	isFailed := false
	isRunning := false
	if status != "" {
		statusLabel = strings.ToUpper(status)
		switch strings.ToLower(status) {
		case "completed", "success", "ok":
			statusBg = s.theme.Colors.SuccessContainer
			statusFg = s.theme.Colors.OnSuccessContainer
		case "running", "in_progress":
			statusBg = s.theme.Colors.PrimaryContainer
			statusFg = s.theme.Colors.OnPrimaryContainer
			isRunning = true
		case "failed", "error":
			statusBg = s.theme.Colors.ErrorContainer
			statusFg = s.theme.Colors.OnErrorContainer
			isFailed = true
		}
	}

	if diff := s.toolDiffInfo(toolKey, item.Text); diff.isDiff {
		return s.layoutDiffToolItem(gtx, cacheKey, toolKey, item, diff, statusLabel, statusBg, statusFg)
	}

	expanded, hasExplicit := s.toolExpanded[toolKey]
	if !hasExplicit {
		expanded = isRunning || isFailed
	}

	chevron := uikit.KindChevronRight
	if expanded {
		chevron = uikit.KindChevronDown
	}

	icon := toolCategoryIcon(item.Title)

	headerBg := s.theme.Colors.SurfaceContainerHigh
	headerBorder := s.theme.Colors.OutlineVariant
	if btn.Hovered() {
		headerBg = s.theme.Colors.SurfaceContainerHighest
		headerBorder = s.theme.Colors.Primary
	}

	return uikit.Inset{Top: 4, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(36)
					return s.roundedBorderSurface(gtx, shapeMedium, headerBg, headerBorder, 1, func(gtx layout.Context) layout.Dimensions {
						semantic.DescriptionOp(s.conversationDescription(cacheKey, item)).Add(gtx.Ops)
						return uikit.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.LayoutActionIcon(gtx, icon, 16, s.theme.Colors.Primary)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, item.Title, textBodyMedium, font.SemiBold, s.theme.Colors.OnSurface, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, statusBg, func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, statusLabel, textLabelSmall, font.Bold, statusFg, 1)
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return uikit.LayoutActionIcon(gtx, chevron, 12, s.theme.Colors.OnSurfaceVariant)
									})
								}),
							)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !expanded || strings.TrimSpace(item.Text) == "" {
					return layout.Dimensions{}
				}
				return s.layoutToolOutputBody(gtx, cacheKey, toolKey, item.Text)
			}),
		)
	})
}

type toolOutputPageButtons struct {
	previous widget.Clickable
	next     widget.Clickable
	collapse widget.Clickable
}

func (s *Shell) toolOutputPageState(key conversationCacheKey) *toolOutputPageButtons {
	if s.toolOutputPages == nil {
		s.toolOutputPages = make(map[conversationCacheKey]*toolOutputPageButtons)
	}
	pages := s.toolOutputPages[key]
	if pages == nil {
		s.admitExpansionKey(key)
		pages = new(toolOutputPageButtons)
		s.toolOutputPages[key] = pages
	}
	return pages
}

func toolOutputWindow(source string, page int) (text string, pageCount, current int) {
	return conversationcomponent.ToolOutputWindow(source, page)
}

func (s *Shell) layoutToolOutputBody(gtx layout.Context, key conversationCacheKey, toolKey, text string) layout.Dimensions {
	if len(text) <= maxToolOutputUnpagedBytes {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Spacer{}.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutMessageCopyButton(gtx, toolKey+":copy", text)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, text, textBodySmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 0)
				})
			}),
		)
	}

	pages := s.toolOutputPageState(key)
	expanded := s.conversationExpanded[key]
	if !expanded {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, largeMessagePreview(text), textBodySmall, font.Normal, s.theme.Colors.OnSurfaceVariant, largeMessagePreviewLines)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &pages.collapse, "Show full output", true, func() {
					s.conversationExpanded[key] = true
				})
			}),
		)
	}

	page := 0
	if stored, ok := s.conversationPage[key]; ok {
		page = stored
	}
	pageText, pageCount, current := toolOutputWindow(text, page)
	s.conversationPage[key] = current
	copyText := text
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMessageCopyButton(gtx, toolKey+":copy", copyText)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, pageText, textBodySmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "Part " + strconv.Itoa(current+1) + " of " + strconv.Itoa(pageCount)
			return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, s.theme.Colors.OnSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutButton(gtx, &pages.previous, "Previous part", current > 0, func() {
						if page, ok := s.conversationPage[key]; ok && page > 0 {
							s.conversationPage[key] = page - 1
						}
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutButton(gtx, &pages.next, "Next part", current < pageCount-1, func() {
						s.conversationPage[key] = current + 1
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutButton(gtx, &pages.collapse, "Show less", true, func() {
							s.conversationExpanded[key] = false
							s.conversationPage[key] = 0
						})
					})
				}),
			)
		}),
	)
}

func (s *Shell) layoutDiffToolItem(gtx layout.Context, cacheKey conversationCacheKey, toolKey string, item desktopstate.TimelineItem, diff toolDiffCacheEntry, statusLabel string, statusBg, statusFg color.NRGBA) layout.Dimensions {
	adds, dels := diff.additions, diff.deletions
	displayTitle := item.Title
	if diff.filename != "" {
		displayTitle = diff.filename
	}

	btn, ok := s.toolExpandButtons[toolKey]
	if !ok {
		btn = new(widget.Clickable)
		s.toolExpandButtons[toolKey] = btn
	}
	if btn.Clicked(gtx) {
		s.toolExpanded[toolKey] = !s.toolExpanded[toolKey]
	}

	expanded, hasExplicit := s.toolExpanded[toolKey]
	if !hasExplicit {
		// Diffs default collapsed like plain tool items: expanded previews
		// re-render their line list on every frame while scrolling through
		// history, which dominated frame cost in diff-heavy sessions.
		expanded = false
	}

	chevron := uikit.KindChevronRight
	if expanded {
		chevron = uikit.KindChevronDown
	}

	headerBg := s.theme.Colors.SurfaceContainerHigh
	headerBorder := s.theme.Colors.OutlineVariant
	if btn.Hovered() {
		headerBg = s.theme.Colors.SurfaceContainerHighest
		headerBorder = s.theme.Colors.Primary
	}

	previewLines, omitted := diff.preview, diff.omitted

	return uikit.Inset{Top: 4, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(36)
					return s.roundedBorderSurface(gtx, shapeMedium, headerBg, headerBorder, 1, func(gtx layout.Context) layout.Dimensions {
						semantic.DescriptionOp(s.conversationDescription(cacheKey, item)).Add(gtx.Ops)
						return uikit.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.LayoutActionIcon(gtx, uikit.KindCompose, 16, s.theme.Colors.Primary)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, displayTitle, textBodyMedium, font.SemiBold, s.theme.Colors.OnSurface, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if adds == 0 {
										return layout.Dimensions{}
									}
									return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.DiffAddedContainer, func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, fmt.Sprintf("+%d", adds), textLabelSmall, font.Bold, s.theme.Colors.DiffAdded, 1)
											})
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if dels == 0 {
										return layout.Dimensions{}
									}
									return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.DiffDeletedContainer, func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, fmt.Sprintf("-%d", dels), textLabelSmall, font.Bold, s.theme.Colors.DiffDeleted, 1)
											})
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, statusBg, func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, statusLabel, textLabelSmall, font.Bold, statusFg, 1)
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return uikit.LayoutActionIcon(gtx, chevron, 12, s.theme.Colors.OnSurfaceVariant)
									})
								}),
							)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !expanded || len(previewLines) == 0 {
					return layout.Dimensions{}
				}
				return uikit.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedBorderSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLowest, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lineChildren := make([]layout.FlexChild, 0, len(previewLines)+2)
							lineChildren = append(lineChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.Spacer{}.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutMessageCopyButton(gtx, toolKey+":diffcopy", item.Text)
									}),
								)
							}))
							for _, line := range previewLines {
								lineStr := line
								lineChildren = append(lineChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutDiffLine(gtx, lineStr)
								}))
							}
							if omitted > 0 {
								lineChildren = append(lineChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, fmt.Sprintf("… %d more lines omitted", omitted), textLabelSmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 1)
									})
								}))
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lineChildren...)
						})
					})
				})
			}),
		)
	})
}

func (s *Shell) layoutDiffLine(gtx layout.Context, line string) layout.Dimensions {
	lineColor := s.theme.Colors.OnSurfaceVariant
	lineBg := color.NRGBA{}
	trimmed := strings.TrimSpace(line)

	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		lineColor = s.theme.Colors.DiffAdded
		lineBg = s.theme.Colors.DiffAddedContainer
	} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		lineColor = s.theme.Colors.DiffDeleted
		lineBg = s.theme.Colors.DiffDeletedContainer
	} else if strings.HasPrefix(trimmed, "@@") {
		lineColor = s.theme.Colors.Tertiary
	}

	content := func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 1, Bottom: 1, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, line, textBodySmall, font.Normal, lineColor, 1)
		})
	}

	if lineBg.A > 0 {
		return s.roundedSurface(gtx, shapeSmall, lineBg, content)
	}
	return content(gtx)
}

func (s *Shell) layoutSubagentItem(gtx layout.Context, subagent desktopstate.SubagentState) layout.Dimensions {
	profile := strings.ToLower(strings.TrimSpace(subagent.Profile))
	badgeText := "SUBAGENT"
	badgeBg := s.theme.Colors.SecondaryContainer
	badgeFg := s.theme.Colors.OnSecondaryContainer
	accentColor := s.theme.Colors.Secondary

	switch profile {
	case "strength", "str":
		badgeText = "STR · STRENGTH"
		badgeBg = s.theme.Colors.StrengthContainer
		badgeFg = s.theme.Colors.OnStrengthContainer
		accentColor = s.theme.Colors.Strength
	case "agility", "agi":
		badgeText = "AGI · AGILITY"
		badgeBg = s.theme.Colors.AgilityContainer
		badgeFg = s.theme.Colors.OnAgilityContainer
		accentColor = s.theme.Colors.Agility
	case "intelligence", "int":
		badgeText = "INT · INTELLIGENCE"
		badgeBg = s.theme.Colors.IntelligenceContainer
		badgeFg = s.theme.Colors.OnIntelligenceContainer
		accentColor = s.theme.Colors.Intelligence
	default:
		if profile != "" {
			badgeText = "" + strings.ToUpper(profile)
		}
	}

	status := strings.ToUpper(strings.TrimSpace(subagent.Status))
	if status == "" {
		status = "PENDING"
	}
	statusBg := s.theme.Colors.SurfaceContainerLow
	statusFg := s.theme.Colors.OnSurfaceVariant
	switch strings.ToLower(status) {
	case "completed", "integrated":
		statusBg = s.theme.Colors.SuccessContainer
		statusFg = s.theme.Colors.OnSuccessContainer
	case "running", "farming", "roaming", "sticking":
		statusBg = s.theme.Colors.PrimaryContainer
		statusFg = s.theme.Colors.OnPrimaryContainer
	case "failed", "b", "care":
		statusBg = s.theme.Colors.ErrorContainer
		statusFg = s.theme.Colors.OnErrorContainer
	}

	return uikit.Inset{Top: 6, Bottom: 6, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeLarge, s.theme.Colors.SurfaceContainerHigh, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp("Delegated agent " + subagent.Profile + ", " + subagent.Status + ", " + subagent.Task).Add(gtx.Ops)
			return layout.Stack{Alignment: layout.W}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 12, Bottom: 12, Left: 18, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, badgeBg, func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, badgeText, textLabelMedium, font.Bold, badgeFg, 1)
											})
										})
									}),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.Spacer{}.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, statusBg, func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, status, textLabelSmall, font.SemiBold, statusFg, 1)
											})
										})
									}),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if strings.TrimSpace(subagent.Task) == "" {
									return layout.Dimensions{}
								}
								return uikit.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, subagent.Task, textBodyMedium, font.Medium, s.theme.Colors.OnSurface, 3)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if strings.TrimSpace(subagent.Summary) == "" {
									return layout.Dimensions{}
								}
								return uikit.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, s.theme.Colors.SurfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, subagent.Summary, textBodySmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 6)
										})
									})
								})
							}),
						)
					})
				}),
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					stripeWidth := gtx.Dp(4)
					cornerRadius := gtx.Dp(shapeLarge)
					stack := clip.RRect{
						Rect: image.Rectangle{Max: image.Point{X: stripeWidth, Y: gtx.Constraints.Max.Y}},
						NW:   cornerRadius,
						SW:   cornerRadius,
					}.Push(gtx.Ops)
					paint.Fill(gtx.Ops, accentColor)
					stack.Pop()
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
			)
		})
	})
}
