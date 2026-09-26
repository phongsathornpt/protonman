//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	inspectorWideBreakpoint unit.Dp = 820
	inspectorPanelWidth     unit.Dp = 336
)

var (
	reasoningChoices      = []string{"auto", "none", "minimal", "low", "medium", "high", "xhigh", "max"}
	lowConcurrencyChoices = []string{"auto", "on", "off"}
)

func (s *shell) shouldShowInspector(wide bool) bool {
	if s.inspectorOverride {
		return s.inspectorVisible
	}
	return wide
}

func (s *shell) syncRuntimeEditors(state desktopstate.State) {
	session, ok := selectedSession(state)
	if !ok {
		if s.runtimeEditorKey != "" {
			s.runtimeEditorKey = ""
			s.runtimeProviderEditor.SetText("")
			s.runtimeModelEditor.SetText("")
		}
		return
	}
	key := session.ID + "\x00" + session.Runtime.Provider + "\x00" + session.Runtime.Model + "\x00" + session.Runtime.Reasoning + "\x00" + session.Runtime.LowConcurrency
	if key == s.runtimeEditorKey {
		return
	}
	s.runtimeEditorKey = key
	s.runtimeProviderEditor.SetText(session.Runtime.Provider)
	s.runtimeModelEditor.SetText(session.Runtime.Model)
}

func (s *shell) layoutInspector(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot, compact bool) layout.Dimensions {
	if compact {
		gtx.Constraints.Min.X = 0
	} else {
		gtx.Constraints.Min.X = gtx.Dp(inspectorPanelWidth)
		gtx.Constraints.Max.X = gtx.Dp(inspectorPanelWidth)
	}
	panelCount := 6
	if !protonmanSession(session) {
		panelCount = 3
	}
	return s.roundedSurface(gtx, shapeLarge, s.theme.surfaceContainer, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutInspectorHeader(gtx, session, snapshot)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return s.inspectorList.Layout(gtx, panelCount, func(gtx layout.Context, index int) layout.Dimensions {
					return s.layoutInspectorPanel(gtx, session, snapshot, index)
				})
			}),
		)
	})
}

func (s *shell) layoutInspectorHeader(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(64)
	return desktopInset{Top: 12, Bottom: 12, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "SESSION CONTEXT", textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				status := "Agent connected"
				if sessionConnection(snapshot, session.AgentID) != connectionConnected {
					status = "Waiting for agent"
				}
				return s.layoutLabel(gtx, status, textBodyMedium, font.Normal, s.theme.onSurface, 1)
			}),
		)
	})
}

func (s *shell) layoutInspectorPanel(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot, index int) layout.Dimensions {
	var content layout.Widget
	if !protonmanSession(session) {
		switch index {
		case 0:
			content = func(gtx layout.Context) layout.Dimensions {
				return s.layoutExternalAgentPanel(gtx, session, snapshot)
			}
		case 1:
			content = func(gtx layout.Context) layout.Dimensions {
				return s.layoutMCPIntegrationsPanel(gtx, snapshot)
			}
		default:
			content = func(gtx layout.Context) layout.Dimensions {
				return s.layoutAgentProfilesPanel(gtx, snapshot)
			}
		}
		return s.layoutInspectorPanelSurface(gtx, content)
	}
	switch index {
	case 0:
		content = func(gtx layout.Context) layout.Dimensions {
			return s.layoutGoalPanel(gtx, session)
		}
	case 1:
		content = func(gtx layout.Context) layout.Dimensions {
			return s.layoutTodoPanel(gtx, session)
		}
	case 2:
		content = func(gtx layout.Context) layout.Dimensions {
			return s.layoutMemoryPanel(gtx, session)
		}
	case 3:
		content = func(gtx layout.Context) layout.Dimensions {
			return s.layoutRuntimePanel(gtx, session, snapshot)
		}
	case 4:
		content = func(gtx layout.Context) layout.Dimensions {
			return s.layoutMCPIntegrationsPanel(gtx, snapshot)
		}
	default:
		content = func(gtx layout.Context) layout.Dimensions {
			return s.layoutAgentProfilesPanel(gtx, snapshot)
		}
	}
	return s.layoutInspectorPanelSurface(gtx, content)
}

func (s *shell) layoutInspectorPanelSurface(gtx layout.Context, content layout.Widget) layout.Dimensions {
	return desktopUniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp("Session inspector panel").Add(gtx.Ops)
			return desktopInset{Top: 16, Bottom: 16, Left: 16, Right: 16}.Layout(gtx, content)
		})
	})
}

func (s *shell) layoutExternalAgentPanel(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPanelTitle(gtx, "Agent capabilities")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "This conversation uses standard ACP chat, tools, permissions, cancellation, and reconnect behavior through "+agentDisplayName(snapshot.AgentProfiles, session.AgentID)+".", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 4)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "Goal, TODO, Memory, and runtime controls are hidden because they are Protonman extensions.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 4)
		}),
	)
}

func protonmanSession(session desktopstate.SessionState) bool {
	return session.AgentID == "" || session.AgentID == controllerAgentID
}

func (s *shell) layoutGoalPanel(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	goal := strings.TrimSpace(session.Context.Goal)
	hasGoal := goal != ""
	if !hasGoal {
		goal = "No active goal set"
	}
	goal = compactInspectorText(goal, 1200)
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutPanelTitle(gtx, "Active Goal")
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasGoal {
						return layout.Dimensions{}
					}
					return s.roundedSurface(gtx, shapeSmall, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "ACTIVE", textLabelSmall, font.Bold, s.theme.onPrimaryContainer, 1)
						})
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg := s.theme.surfaceContainerLowest
			borderColor := s.theme.outlineVariant
			textColor := s.theme.onSurface
			if hasGoal {
				borderColor = s.theme.primary
			} else {
				textColor = s.theme.onSurfaceVariant
			}
			return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedBorderSurface(gtx, shapeMedium, bg, borderColor, 1, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, goal, textBodyMedium, font.Normal, textColor, 8)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								tip := "Goals are managed by the connected agent."
								if hasGoal {
									tip = "Directs autonomous planning, subagents, and task plans."
								}
								return desktopInset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, tip, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 2)
								})
							}),
						)
					})
				})
			})
		}),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutTodoPanel(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	todo := session.Context.Todo
	title := "Task Plan"
	revisionText := ""
	if todo.Revision > 0 {
		revisionText = fmt.Sprintf("rev %d", todo.Revision)
	}

	completedCount := 0
	for _, item := range todo.Items {
		if strings.TrimSpace(item.Status) == "completed" {
			completedCount++
		}
	}
	totalCount := len(todo.Items)

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutPanelTitle(gtx, title)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if revisionText == "" {
						return layout.Dimensions{}
					}
					return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, revisionText, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
						})
					})
				}),
			)
		}),
	}

	if totalCount > 0 {
		percent := (completedCount * 100) / totalCount
		progressText := fmt.Sprintf("%d of %d completed (%d%%)", completedCount, totalCount, percent)
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 6, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, progressText, textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = gtx.Dp(6)
								gtx.Constraints.Max.Y = gtx.Dp(6)
								return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
									fillWidth := (gtx.Constraints.Max.X * percent) / 100
									if fillWidth < gtx.Dp(6) && completedCount > 0 {
										fillWidth = gtx.Dp(6)
									}
									return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											gtx.Constraints.Min.X = fillWidth
											gtx.Constraints.Max.X = fillWidth
											return s.roundedSurface(gtx, shapeSmall, s.theme.primary, func(gtx layout.Context) layout.Dimensions {
												return layout.Dimensions{Size: gtx.Constraints.Min}
											})
										}),
									)
								})
							})
						}),
					)
				})
			}),
		)
		for _, item := range todo.Items {
			item := item
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutTodoItem(gtx, item)
			}))
		}
	} else {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "No task plan active. Protonman coordinates tasks automatically using session TODOs.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 3)
					})
				})
			})
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutTodoItem(gtx layout.Context, item desktopstate.TodoItemState) layout.Dimensions {
	status := strings.TrimSpace(item.Status)
	marker := todoMarkerForStatus(status)
	text := strings.TrimSpace(item.Text)
	if text == "" {
		text = "Untitled task"
	}
	text = compactInspectorText(text, 500)
	description := "TODO " + marker + " " + text
	if status != "" {
		description += ", " + status
	}
	semantic.DescriptionOp(description).Add(gtx.Ops)

	markerColor := s.theme.outlineVariant
	textColor := s.theme.onSurface
	rowBg := s.theme.surfaceContainerLowest
	borderColor := s.theme.outlineVariant
	borderWidth := 1

	switch status {
	case "completed":
		markerColor = s.theme.onSuccessContainer
		textColor = s.theme.onSurfaceVariant
	case "in_progress":
		markerColor = s.theme.primary
		textColor = s.theme.onSurface
		borderColor = s.theme.primary
	}

	return desktopInset{Top: 3, Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeSmall, rowBg, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(24)
						return s.layoutLabel(gtx, marker, textTitleMedium, font.Bold, markerColor, 1)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, text, textBodyMedium, font.Normal, textColor, 3)
					}),
				)
			})
		})
	})
}

func (s *shell) layoutMemoryPanel(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	memory := session.Context.Memory
	totalCount := len(memory.Workspace) + len(memory.Global)
	countLabel := fmt.Sprintf("%d stored", totalCount)

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutPanelTitle(gtx, "Durable Memory")
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, countLabel, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
						})
					})
				}),
			)
		}),
	}
	if totalCount == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "No workspace or global memory. Protonman captures durable facts and user preferences as you work.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 3)
					})
				})
			})
		}))
	} else {
		if len(memory.Workspace) > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutMemorySection(gtx, "Workspace Facts", memory.Workspace)
			}))
		}
		if len(memory.Global) > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutMemorySection(gtx, "Global Preferences", memory.Global)
			}))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutMemorySection(gtx layout.Context, title string, entries []desktopstate.MemoryEntryState) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, strings.ToUpper(title), textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
		}),
	}
	for _, entry := range entries {
		entry := entry
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutMemoryEntry(gtx, entry)
		}))
	}
	return desktopInset{Top: 6, Bottom: 4, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (s *shell) layoutMemoryEntry(gtx layout.Context, entry desktopstate.MemoryEntryState) layout.Dimensions {
	key := strings.TrimSpace(entry.Key)
	value := strings.TrimSpace(entry.Value)
	if key == "" {
		key = strings.TrimSpace(entry.ID)
	}
	key = compactInspectorText(key, 240)
	value = compactInspectorText(value, 1200)
	if key == "" && value == "" {
		return layout.Dimensions{}
	}
	description := "Memory "
	if entry.Kind != "" {
		description += entry.Kind + " "
	}
	description += key
	if value != "" {
		description += ": " + value
	}
	semantic.DescriptionOp(description).Add(gtx.Ops)

	return desktopInset{Top: 3, Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						kindLabel := strings.ToUpper(entry.Kind)
						if kindLabel == "" {
							kindLabel = "FACT"
						}
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, s.theme.secondaryContainer, func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, kindLabel, textLabelSmall, font.Bold, s.theme.onSecondaryContainer, 1)
									})
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, key, textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
								})
							}),
						)
					}),
				}
				if value != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, value, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 5)
						})
					}))
				}
				if metadata := memoryMetadata(entry); metadata != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "● "+metadata, textLabelSmall, font.Normal, s.theme.outline, 1)
						})
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func memoryMetadata(entry desktopstate.MemoryEntryState) string {
	parts := make([]string, 0, 3)
	if entry.Confidence > 0 {
		parts = append(parts, fmt.Sprintf("%.0f%% confidence", entry.Confidence*100))
	}
	if entry.UsageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d uses", entry.UsageCount))
	}
	return strings.Join(parts, " · ")
}

func (s *shell) layoutRuntimePanel(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	busy := sessionBusy(session.Status)
	enabled := sessionConnection(snapshot, session.AgentID) == connectionConnected && !busy && !snapshot.RuntimeUpdating
	s.runtimeProviderEditor.ReadOnly = !enabled
	s.runtimeModelEditor.ReadOnly = !enabled
	for enabled {
		if _, ok := s.runtimeProviderEditor.Update(gtx); !ok {
			break
		}
	}
	for enabled {
		if _, ok := s.runtimeModelEditor.Update(gtx); !ok {
			break
		}
	}
	status := "Changes apply to future turns"
	if snapshot.RuntimeUpdating {
		status = "Updating runtime…"
	} else if !enabled {
		status = "Runtime controls are read-only while the session is busy"
	}
	provider := strings.TrimSpace(s.runtimeProviderEditor.Text())
	model := strings.TrimSpace(s.runtimeModelEditor.Text())
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPanelTitle(gtx, "Model & Reasoning")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutInspectorEditor(gtx, "Provider", &s.runtimeProviderEditor, enabled)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutInspectorEditor(gtx, "Model", &s.runtimeModelEditor, enabled)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopUniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutPrimaryButton(gtx, &s.runtimeApplyButton, "Apply model", enabled && provider != "" && model != "", func() {
					s.onSetRuntimeModel(provider, model)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "Reasoning", textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutChoiceGrid(gtx, reasoningChoices, session.Runtime.Reasoning, s.reasoningButtons, enabled, s.onSetRuntimeReasoning)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "Low concurrency", textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutChoiceGrid(gtx, lowConcurrencyChoices, session.Runtime.LowConcurrency, s.lowConcurrencyButtons, enabled, s.onSetRuntimeLow)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, status, textLabelMedium, font.Normal, s.theme.onSurfaceVariant, 2)
		}),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutPanelTitle(gtx layout.Context, title string) layout.Dimensions {
	return s.layoutLabel(gtx, title, textTitleMedium, font.SemiBold, s.theme.onSurface, 2)
}

func (s *shell) layoutInspectorEditor(gtx layout.Context, label string, editor *widget.Editor, enabled bool) layout.Dimensions {
	textColor := s.theme.onSurface
	if !enabled {
		textColor = s.theme.onSurfaceVariant
	}
	textMaterial := op.Record(gtx.Ops)
	paint.ColorOp{Color: textColor}.Add(gtx.Ops)
	textCall := textMaterial.Stop()
	selectionMaterial := op.Record(gtx.Ops)
	paint.ColorOp{Color: s.theme.primaryContainer}.Add(gtx.Ops)
	selectionCall := selectionMaterial.Stop()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(44)
			semantic.EnabledOp(enabled).Add(gtx.Ops)
			semantic.DescriptionOp(label).Add(gtx.Ops)
			dims := s.roundedSurface(gtx, shapeSmall, s.theme.surface, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(44)
				return desktopUniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return editor.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(font.Normal), textBodyMedium, textCall, selectionCall)
				})
			})
			if enabled && gtx.Focused(editor) {
				widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: dims.Size}
				})
			} else {
				widget.Border{Color: s.theme.outlineVariant, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: dims.Size}
				})
			}
			return dims
		}),
	)
}

func (s *shell) layoutChoiceGrid(gtx layout.Context, values []string, selected string, buttons map[string]*widget.Clickable, enabled bool, action func(string)) layout.Dimensions {
	children := make([]layout.FlexChild, 0, (len(values)+1)/2)
	for index := 0; index < len(values); index += 2 {
		first := values[index]
		second := ""
		if index+1 < len(values) {
			second = values[index+1]
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 3, Bottom: 3, Left: 3, Right: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return s.layoutChoiceButton(gtx, first, first == selected, buttons, enabled, action)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if second == "" {
							return layout.Dimensions{}
						}
						return s.layoutChoiceButton(gtx, second, second == selected, buttons, enabled, action)
					}),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutChoiceButton(gtx layout.Context, value string, selected bool, buttons map[string]*widget.Clickable, enabled bool, action func(string)) layout.Dimensions {
	button := buttons[value]
	if button == nil {
		button = new(widget.Clickable)
		buttons[value] = button
	}
	if enabled && button.Clicked(gtx) && action != nil {
		action(value)
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	gtx.Constraints.Min.Y = gtx.Dp(44)
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(44)
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(gtx.Enabled()).Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		semantic.DescriptionOp("Set " + value).Add(gtx.Ops)
		background := s.theme.surface
		foreground := s.theme.onSurface
		if selected {
			background = s.theme.primaryContainer
			foreground = s.theme.onPrimaryContainer
		} else if button.Hovered() && gtx.Enabled() {
			background = s.theme.primaryContainer
			foreground = s.theme.onPrimaryContainer
		}
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Bottom: 8, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, value, textLabelMedium, font.SemiBold, foreground, 1)
			})
		})
	})
	if enabled && gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func todoMarkerForStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done":
		return "✓"
	case "in_progress", "in-progress", "doing":
		return "◐"
	default:
		return "○"
	}
}

func compactInspectorText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value
	}
	runeCount := 0
	prefixEnd := 0
	for index := 0; index < len(value); {
		if runeCount == maxRunes {
			if maxRunes == 1 {
				return "…"
			}
			return strings.TrimSpace(value[:prefixEnd]) + "…"
		}
		_, width := utf8.DecodeRuneInString(value[index:])
		index += width
		runeCount++
		if runeCount == maxRunes-1 {
			prefixEnd = index
		}
	}
	return value
}
