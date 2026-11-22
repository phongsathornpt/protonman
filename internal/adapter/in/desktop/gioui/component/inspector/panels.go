//go:build desktop || desktop_gio

package inspector

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
)

type TodoItem struct{ Status, Text string }
type Todo struct {
	Revision uint64
	Items    []TodoItem
}
type MemoryEntry struct {
	ID, Kind, Key, Value string
	Confidence           float64
	UsageCount           uint64
}
type Memory struct{ Workspace, Global []MemoryEntry }

func (c *Component) LayoutGoal(gtx layout.Context, goal string, chrome Chrome) layout.Dimensions {
	goal = strings.TrimSpace(goal)
	hasGoal := goal != ""
	if !hasGoal {
		goal = "No active goal set"
	}
	goal = compactText(goal, 1200)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return panelTitle(gtx, chrome, "Active Goal") }),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasGoal {
						return layout.Dimensions{}
					}
					return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, "ACTIVE", unit.Sp(11), font.Bold, chrome.Colors.OnPrimaryContainer, 1)
						})
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg, border, textColor := chrome.Colors.SurfaceContainerLowest, chrome.Colors.OutlineVariant, chrome.Colors.OnSurface
			if hasGoal {
				border = chrome.Colors.Primary
			} else {
				textColor = chrome.Colors.OnSurfaceVariant
			}
			return chrome.Inset(gtx, layout.Inset{Top: 8}, func(gtx layout.Context) layout.Dimensions {
				return chrome.BorderSurface(gtx, unit.Dp(8), bg, border, 1, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, goal, unit.Sp(14), font.Normal, textColor, 8)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								tip := "Goals are managed by the connected agent."
								if hasGoal {
									tip = "Directs autonomous planning, subagents, and task plans."
								}
								return chrome.Inset(gtx, layout.Inset{Top: 6}, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, tip, unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 2)
								})
							}),
						)
					})
				})
			})
		}),
	)
}

func (c *Component) LayoutTodo(gtx layout.Context, todo Todo, chrome Chrome) layout.Dimensions {
	completed := 0
	for _, item := range todo.Items {
		if strings.TrimSpace(item.Status) == "completed" {
			completed++
		}
	}
	revision := ""
	if todo.Revision > 0 {
		revision = fmt.Sprintf("rev %d", todo.Revision)
	}
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return panelTitle(gtx, chrome, "Task Plan") }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if revision == "" {
					return layout.Dimensions{}
				}
				return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, revision, unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
					})
				})
			}),
		)
	})}
	if len(todo.Items) == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return emptyPanel(gtx, chrome, "No task plan active. Protonman coordinates tasks automatically using session TODOs.", 3)
		}))
	} else {
		percent := completed * 100 / len(todo.Items)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, fmt.Sprintf("%d of %d completed (%d%%)", completed, len(todo.Items), percent), unit.Sp(11), font.Medium, chrome.Colors.OnSurfaceVariant, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(6), gtx.Dp(6)
							return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
								width := gtx.Constraints.Max.X * percent / 100
								if width < gtx.Dp(6) && completed > 0 {
									width = gtx.Dp(6)
								}
								gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
								return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.Primary, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} })
							})
						})
					}),
				)
			})
		}))
		for _, item := range todo.Items {
			item := item
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layoutTodoItem(gtx, item, chrome) }))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func layoutTodoItem(gtx layout.Context, item TodoItem, chrome Chrome) layout.Dimensions {
	status := strings.TrimSpace(item.Status)
	marker, textColor, border := TodoMarker(status), chrome.Colors.OnSurface, chrome.Colors.OutlineVariant
	if marker == "✓" {
		textColor = chrome.Colors.OnSurfaceVariant
	} else if marker == "◐" {
		border = chrome.Colors.Primary
	}
	text := compactText(strings.TrimSpace(item.Text), 500)
	if text == "" {
		text = "Untitled task"
	}
	description := "TODO " + marker + " " + text
	if status != "" {
		description += ", " + status
	}
	semantic.DescriptionOp(description).Add(gtx.Ops)
	return chrome.Inset(gtx, layout.Inset{Top: 3, Bottom: 3}, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLowest, border, 1, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(24)
						markerColor := chrome.Colors.OutlineVariant
						if marker == "✓" {
							markerColor = chrome.Colors.OnSuccessContainer
						}
						if marker == "◐" {
							markerColor = chrome.Colors.Primary
						}
						return chrome.Label(gtx, marker, unit.Sp(16), font.Bold, markerColor, 1)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, text, unit.Sp(14), font.Normal, textColor, 3)
					}),
				)
			})
		})
	})
}

func (c *Component) LayoutMemory(gtx layout.Context, memory Memory, chrome Chrome) layout.Dimensions {
	total := len(memory.Workspace) + len(memory.Global)
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return panelTitle(gtx, chrome, "Durable Memory") }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, fmt.Sprintf("%d stored", total), unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
					})
				})
			}),
		)
	})}
	if total == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return emptyPanel(gtx, chrome, "No workspace or global memory. Protonman captures durable facts and user preferences as you work.", 3)
		}))
	} else {
		if len(memory.Workspace) > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return memorySection(gtx, "Workspace Facts", memory.Workspace, chrome)
			}))
		}
		if len(memory.Global) > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return memorySection(gtx, "Global Preferences", memory.Global, chrome)
			}))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func memorySection(gtx layout.Context, title string, entries []MemoryEntry, chrome Chrome) layout.Dimensions {
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return chrome.Label(gtx, strings.ToUpper(title), unit.Sp(12), font.SemiBold, chrome.Colors.OnSurfaceVariant, 1)
	})}
	for _, entry := range entries {
		entry := entry
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return memoryEntry(gtx, entry, chrome) }))
	}
	return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 4, Left: 4, Right: 4}, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func memoryEntry(gtx layout.Context, entry MemoryEntry, chrome Chrome) layout.Dimensions {
	key, value := strings.TrimSpace(entry.Key), compactText(strings.TrimSpace(entry.Value), 1200)
	if key == "" {
		key = strings.TrimSpace(entry.ID)
	}
	key = compactText(key, 240)
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
	kind := strings.ToUpper(entry.Kind)
	if kind == "" {
		kind = "FACT"
	}
	return chrome.Inset(gtx, layout.Inset{Top: 3, Bottom: 3}, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLowest, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SecondaryContainer, func(gtx layout.Context) layout.Dimensions {
								return chrome.Inset(gtx, layout.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, kind, unit.Sp(11), font.Bold, chrome.Colors.OnSecondaryContainer, 1)
								})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Left: 6}, func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, key, unit.Sp(14), font.SemiBold, chrome.Colors.OnSurface, 1)
							})
						}),
					)
				})}
				if value != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, value, unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 5)
						})
					}))
				}
				metadata := memoryMetadata(entry)
				if metadata != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, "● "+metadata, unit.Sp(11), font.Normal, chrome.Colors.Outline, 1)
						})
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func memoryMetadata(entry MemoryEntry) string {
	parts := make([]string, 0, 2)
	if entry.Confidence > 0 {
		parts = append(parts, fmt.Sprintf("%.0f%% confidence", entry.Confidence*100))
	}
	if entry.UsageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d uses", entry.UsageCount))
	}
	return strings.Join(parts, " · ")
}

func panelTitle(gtx layout.Context, chrome Chrome, title string) layout.Dimensions {
	return chrome.Label(gtx, title, unit.Sp(16), font.SemiBold, chrome.Colors.OnSurface, 2)
}

func emptyPanel(gtx layout.Context, chrome Chrome, text string, lines int) layout.Dimensions {
	return chrome.Inset(gtx, layout.Inset{Top: 8}, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(8), chrome.Colors.SurfaceContainerLowest, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, text, unit.Sp(14), font.Normal, chrome.Colors.OnSurfaceVariant, lines)
			})
		})
	})
}

func compactText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value
	}
	count, prefixEnd := 0, 0
	for index := 0; index < len(value); {
		if count == maxRunes {
			if maxRunes == 1 {
				return "…"
			}
			return strings.TrimSpace(value[:prefixEnd]) + "…"
		}
		_, width := utf8.DecodeRuneInString(value[index:])
		index += width
		count++
		if count == maxRunes-1 {
			prefixEnd = index
		}
	}
	return value
}

func CompactText(value string, maxRunes int) string { return compactText(value, maxRunes) }

func TodoMarker(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done":
		return "✓"
	case "in_progress", "in-progress", "doing":
		return "◐"
	default:
		return "○"
	}
}

type RuntimePanelInput struct {
	ProviderEditor         *widget.Editor
	ModelEditor            *widget.Editor
	ApplyButton            *widget.Clickable
	ReasoningButtons       map[string]*widget.Clickable
	LowConcurrencyButtons  map[string]*widget.Clickable
	PermissionModeButtons  map[string]*widget.Clickable
	ReasoningChoices       []string
	LowConcurrencyChoices  []string
	PermissionModeChoices  []string
	SelectedReasoning      string
	SelectedLowConcurrency string
	SelectedPermissionMode string
	Enabled                bool
	Updating               bool
	Chrome                 Chrome
	OnSetModel             func(provider, model string)
	OnSetReasoning         func(level string)
	OnSetLowConcurrency    func(val string)
	OnSetPermissionMode    func(mode string)
}

func (c *Component) LayoutRuntime(gtx layout.Context, input RuntimePanelInput) layout.Dimensions {
	chrome := input.Chrome
	enabled := input.Enabled
	for enabled {
		if _, ok := input.ProviderEditor.Update(gtx); !ok {
			break
		}
	}
	for enabled {
		if _, ok := input.ModelEditor.Update(gtx); !ok {
			break
		}
	}
	status := "Changes apply to future turns"
	if input.Updating {
		status = "Updating runtime…"
	} else if !enabled {
		status = "Runtime controls are read-only while the session is busy"
	}
	provider := strings.TrimSpace(input.ProviderEditor.Text())
	model := strings.TrimSpace(input.ModelEditor.Text())

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return panelTitle(gtx, chrome, "Model & Reasoning")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Editor(gtx, "Provider", input.ProviderEditor, enabled)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Editor(gtx, "Model", input.ModelEditor, enabled)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 4, Left: 4, Right: 4}, func(gtx layout.Context) layout.Dimensions {
				return chrome.PrimaryButton(gtx, input.ApplyButton, "Apply model", enabled && provider != "" && model != "", func() {
					if input.OnSetModel != nil {
						input.OnSetModel(provider, model)
					}
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "Reasoning", unit.Sp(12), font.SemiBold, chrome.Colors.OnSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.layoutChoiceGrid(gtx, choiceGridInput{
				Values: input.ReasoningChoices, Selected: input.SelectedReasoning, Buttons: input.ReasoningButtons,
				Enabled: enabled, Chrome: chrome, OnSelect: input.OnSetReasoning,
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "Low concurrency", unit.Sp(12), font.SemiBold, chrome.Colors.OnSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.layoutChoiceGrid(gtx, choiceGridInput{
				Values: input.LowConcurrencyChoices, Selected: input.SelectedLowConcurrency, Buttons: input.LowConcurrencyButtons,
				Enabled: enabled, Chrome: chrome, OnSelect: input.OnSetLowConcurrency,
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "Permission mode", unit.Sp(12), font.SemiBold, chrome.Colors.OnSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			mode := input.SelectedPermissionMode
			if mode == "" {
				mode = "ask"
			}
			return c.layoutChoiceGrid(gtx, choiceGridInput{
				Values: input.PermissionModeChoices, Selected: mode, Buttons: input.PermissionModeButtons,
				Enabled: enabled, Chrome: chrome, OnSelect: input.OnSetPermissionMode,
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, status, unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 2)
		}),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// choiceGridInput groups the state for a grid of mutually exclusive choices.
// The widget map is owned by the caller so selection state survives re-layout.
type choiceGridInput struct {
	Values   []string
	Selected string
	Buttons  map[string]*widget.Clickable
	Enabled  bool
	Chrome   Chrome
	OnSelect func(string)
}

func (c *Component) layoutChoiceGrid(gtx layout.Context, in choiceGridInput) layout.Dimensions {
	chrome := in.Chrome
	children := make([]layout.FlexChild, 0, (len(in.Values)+1)/2)
	for index := 0; index < len(in.Values); index += 2 {
		first := in.Values[index]
		second := ""
		if index+1 < len(in.Values) {
			second = in.Values[index+1]
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 3, Bottom: 3, Left: 3, Right: 3}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return c.layoutChoiceButton(gtx, first, first == in.Selected, in)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if second == "" {
							return layout.Dimensions{}
						}
						return c.layoutChoiceButton(gtx, second, second == in.Selected, in)
					}),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (c *Component) layoutChoiceButton(gtx layout.Context, value string, selected bool, in choiceGridInput) layout.Dimensions {
	button := in.Buttons[value]
	if button == nil {
		button = new(widget.Clickable)
		in.Buttons[value] = button
	}
	if in.Enabled && button.Clicked(gtx) && in.OnSelect != nil {
		in.OnSelect(value)
	}
	if !in.Enabled {
		gtx = gtx.Disabled()
	}
	chrome := in.Chrome
	background := chrome.Colors.Surface
	foreground := chrome.Colors.OnSurface
	border := chrome.Colors.OutlineVariant
	borderWidth := unit.Dp(1)
	if selected {
		background = chrome.Colors.PrimaryContainer
		foreground = chrome.Colors.OnPrimaryContainer
		border = chrome.Colors.Primary
		borderWidth = unit.Dp(2)
	} else if in.Enabled && button.Hovered() {
		background = chrome.Colors.SurfaceContainerHighest
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(6), background, border, int(borderWidth), func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, value, unit.Sp(12), font.Medium, foreground, 1)
			})
		})
	})
}
