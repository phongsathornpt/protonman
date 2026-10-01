//go:build desktop || desktop_gio

package settings

import (
	"image"
	"image/color"
	"log"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/app"
)

type AgentsInput struct {
	Profiles              []app.ACPAgentProfile
	ActiveAgentID         string
	AgentConnections      map[string]string
	AgentStatuses         map[string]string
	AgentError            string
	AgentConfigOverridden bool
	Updating              bool
	Chrome                Chrome
	OnSave                func(origID, newID, name, cmd, args, env string)
	OnRemove              func(id string)
	OnScanDevice          func()
}

func compactText(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

func (c *Component) ClearAgentProfileEditors() {
	c.agent.IDEditor.SetText("")
	c.agent.NameEditor.SetText("")
	c.agent.CommandEditor.SetText("")
	c.agent.ArgsEditor.SetText("")
	c.agent.EnvEditor.SetText("")
	c.agent.ConfirmDelete = false
}

func agentStatusInfo(connection string) (color.NRGBA, string) {
	switch connection {
	case "connected":
		return color.NRGBA{R: 52, G: 199, B: 89, A: 255}, "Ready"
	case "connecting":
		return color.NRGBA{R: 255, G: 159, B: 10, A: 255}, "Starting"
	case "reconnecting":
		return color.NRGBA{R: 255, G: 159, B: 10, A: 255}, "Retrying"
	default:
		return color.NRGBA{R: 142, G: 142, B: 147, A: 255}, "Offline"
	}
}

func (c *Component) LayoutAgents(gtx layout.Context, input AgentsInput) layout.Dimensions {
	profiles := input.Profiles
	enabled := !input.Updating
	chrome := input.Chrome

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.PanelTitle(gtx, "ACP agents")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			summary := "1 supervised daemon process"
			if len(profiles) != 1 {
				summary = strconv.Itoa(len(profiles)) + " supervised daemon processes · Select an agent card to edit"
			}
			return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, summary, unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 2)
			})
		}),
	}

	for _, profile := range profiles {
		profile := profile
		selected := profile.ID == c.agent.EditorOriginalID && c.agent.EditorVisible
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.LayoutAgentProfileRow(gtx, profile, selected, enabled, input)
		}))
	}

	if enabled && c.agent.FormToggleButton.Clicked(gtx) {
		c.agent.EditorVisible = true
		c.agent.EditorOriginalID = ""
		c.agent.EditorKey = ""
		c.agent.ConfirmDelete = false
		c.ClearAgentProfileEditors()
		log.Printf("[UI] + Add ACP Agent clicked, agentEditorVisible is now true")
		gtx.Execute(op.InvalidateCmd{})
	}

	if !c.agent.EditorVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Button(gtx, &c.agent.FormToggleButton, "+ Add ACP Agent", enabled, nil)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Button(gtx, &c.agent.ScanDeviceButton, "Scan Device", enabled, input.OnScanDevice)
						})
					}),
				)
			})
		}))
	}

	if c.agent.EditorVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return c.LayoutAgentConfigurationCard(gtx, input, profiles, enabled)
			})
		}))
	}

	status := "Daemon changes apply upon restarting Protonman Desktop. Environment values are read at launch and never stored."
	statusColor := chrome.Colors.OnSurfaceVariant
	if input.AgentError != "" {
		status = "ACP settings unavailable · " + compactText(input.AgentError, 240)
		statusColor = chrome.Colors.OnErrorContainer
	} else if input.AgentConfigOverridden {
		status = "PROTONMAN_ACP_AGENTS_JSON is active. Remove the environment override to apply changes."
	} else if input.Updating {
		status = "Saving ACP agent settings…"
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return chrome.Inset(gtx, layout.Inset{Top: 8}, func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, status, unit.Sp(11), font.Normal, statusColor, 4)
		})
	}))

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (c *Component) LayoutAgentProfileRow(gtx layout.Context, profile app.ACPAgentProfile, selected, enabled bool, input AgentsInput) layout.Dimensions {
	chrome := input.Chrome
	button := c.agent.ProfileButtons[profile.ID]
	if button == nil {
		button = new(widget.Clickable)
		if c.agent.ProfileButtons == nil {
			c.agent.ProfileButtons = make(map[string]*widget.Clickable)
		}
		c.agent.ProfileButtons[profile.ID] = button
	}
	rowContext := gtx
	if !enabled {
		rowContext = gtx.Disabled()
	}
	if enabled && button.Clicked(rowContext) {
		c.agent.EditorOriginalID = profile.ID
		c.agent.EditorKey = ""
		c.agent.EditorVisible = true
		c.agent.ConfirmDelete = false
		c.agent.IDEditor.SetText(profile.ID)
		c.agent.NameEditor.SetText(profile.DisplayName)
		c.agent.CommandEditor.SetText(profile.Command)
		c.agent.ArgsEditor.SetText(strings.Join(profile.Args, " "))
		c.agent.EnvEditor.SetText(strings.Join(profile.Env, " "))
		log.Printf("[UI] selected agent profile %q for editing", profile.ID)
		gtx.Execute(op.InvalidateCmd{})
	}

	conn := "offline"
	if input.AgentConnections != nil {
		if val, ok := input.AgentConnections[profile.ID]; ok {
			conn = val
		}
	}
	dotColor, statusText := agentStatusInfo(conn)

	isActive := profile.ID == input.ActiveAgentID
	bg := chrome.Colors.SurfaceContainerLow
	borderColor := chrome.Colors.OutlineVariant
	borderWidth := 1
	if selected {
		bg = chrome.Colors.SurfaceContainerHigh
		borderColor = chrome.Colors.Primary
		borderWidth = 2
	} else if enabled && button.Hovered() {
		bg = chrome.Colors.SurfaceContainerHighest
	}

	cmdText := profile.Command
	if len(profile.Args) > 0 {
		cmdText += " " + strings.Join(profile.Args, " ")
	}

	dims := button.Layout(rowContext, func(gtx layout.Context) layout.Dimensions {
		return chrome.Inset(gtx, layout.Inset{Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
			return chrome.BorderSurface(gtx, unit.Dp(8), bg, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
				return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Right: 8}, func(gtx layout.Context) layout.Dimensions {
										size := gtx.Dp(unit.Dp(8))
										gtx.Constraints.Min = image.Pt(size, size)
										gtx.Constraints.Max = image.Pt(size, size)
										paint.FillShape(gtx.Ops, dotColor, clip.Ellipse{Max: image.Pt(size, size)}.Op(gtx.Ops))
										return layout.Dimensions{Size: image.Pt(size, size)}
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, profile.DisplayName, unit.Sp(14), font.SemiBold, chrome.Colors.OnSurface, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
										return chrome.RoundedSurface(gtx, unit.Dp(6), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
											return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
												return chrome.Label(gtx, profile.ID, unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
											})
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if !isActive {
										return layout.Dimensions{}
									}
									return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
										return chrome.RoundedSurface(gtx, unit.Dp(6), chrome.Colors.PrimaryContainer, func(gtx layout.Context) layout.Dimensions {
											return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
												return chrome.Label(gtx, "ACTIVE", unit.Sp(11), font.Bold, chrome.Colors.OnPrimaryContainer, 1)
											})
										})
									})
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{}.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Right: 8}, func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, statusText, unit.Sp(11), font.Medium, dotColor, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.RoundedSurface(gtx, unit.Dp(6), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
										return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
											return chrome.Label(gtx, "Edit", unit.Sp(11), font.SemiBold, chrome.Colors.Primary, 1)
										})
									})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Top: 6}, func(gtx layout.Context) layout.Dimensions {
								return chrome.RoundedSurface(gtx, unit.Dp(6), chrome.Colors.SurfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, compactText(cmdText, 64), unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
									})
								})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if conn == "connected" || input.AgentStatuses == nil {
								return layout.Dimensions{}
							}
							errMsg := input.AgentStatuses[profile.ID]
							if errMsg == "" || errMsg == "Starting "+profile.DisplayName+"…" || errMsg == "Offline" || errMsg == "Connected" {
								return layout.Dimensions{}
							}
							return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
								return chrome.RoundedSurface(gtx, unit.Dp(6), chrome.Colors.ErrorContainer, func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, compactText(errMsg, 80), unit.Sp(11), font.Normal, chrome.Colors.OnErrorContainer, 1)
									})
								})
							})
						}),
					)
				})
			})
		})
	})
	if enabled && rowContext.Focused(button) {
		widget.Border{Color: chrome.Colors.Primary, CornerRadius: unit.Dp(8), Width: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (c *Component) LayoutAgentConfigurationCard(gtx layout.Context, input AgentsInput, profiles []app.ACPAgentProfile, enabled bool) layout.Dimensions {
	chrome := input.Chrome
	isEditing := c.agent.EditorOriginalID != ""
	title := "Add New ACP Agent"
	if isEditing {
		title = "Edit Agent: " + c.agent.NameEditor.Text()
		if strings.TrimSpace(title) == "Edit Agent:" {
			title = "Edit Agent: " + c.agent.EditorOriginalID
		}
	}

	canRemove := enabled && isEditing && len(profiles) > 1
	canSave := enabled && strings.TrimSpace(c.agent.IDEditor.Text()) != "" && strings.TrimSpace(c.agent.CommandEditor.Text()) != ""

	if gtx.Constraints.Max.X > 0 {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
	}
	return chrome.BorderSurface(gtx, unit.Dp(8), chrome.Colors.SurfaceContainerHigh, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		if gtx.Constraints.Max.X > 0 {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
		}
		return chrome.Inset(gtx, layout.Inset{Top: 16, Bottom: 16, Left: 16, Right: 16}, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, title, unit.Sp(16), font.Bold, chrome.Colors.OnSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if c.agent.CloseButton.Clicked(gtx) {
								c.agent.EditorVisible = false
								c.agent.EditorOriginalID = ""
								c.agent.EditorKey = ""
								c.agent.ConfirmDelete = false
								c.ClearAgentProfileEditors()
								gtx.Execute(op.InvalidateCmd{})
							}
							return chrome.MiniIconButton(gtx, &c.agent.CloseButton, "✕", chrome.Colors.OnSurfaceVariant)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if isEditing {
						return layout.Dimensions{}
					}
					return chrome.Inset(gtx, layout.Inset{Top: 10}, func(gtx layout.Context) layout.Dimensions {
						return c.layoutAgentPresetsBar(gtx, enabled, chrome)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Editor(gtx, "Agent ID", &c.agent.IDEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Editor(gtx, "Display name", &c.agent.NameEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Editor(gtx, "Command", &c.agent.CommandEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Editor(gtx, "Arguments (flags or space-separated, e.g. --acp)", &c.agent.ArgsEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 4}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Editor(gtx, "Environment keys (space or comma-separated)", &c.agent.EnvEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if c.agent.ConfirmDelete {
						return chrome.Inset(gtx, layout.Inset{Top: 12}, func(gtx layout.Context) layout.Dimensions {
							return chrome.BorderSurface(gtx, unit.Dp(6), chrome.Colors.ErrorContainer, chrome.Colors.ErrorContainer, 1, func(gtx layout.Context) layout.Dimensions {
								return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return chrome.Label(gtx, "Delete this agent profile? This cannot be undone.", unit.Sp(12), font.SemiBold, chrome.Colors.OnErrorContainer, 2)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return chrome.Inset(gtx, layout.Inset{Top: 8}, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return chrome.Button(gtx, &c.agent.CancelDeleteButton, "Cancel", enabled, func() {
															c.agent.ConfirmDelete = false
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return chrome.DangerButton(gtx, &c.agent.ConfirmDeleteButton, "Confirm Delete", enabled, func() {
															origID := c.agent.EditorOriginalID
															c.agent.EditorVisible = false
															c.agent.EditorOriginalID = ""
															c.agent.EditorKey = ""
															c.agent.ConfirmDelete = false
															c.ClearAgentProfileEditors()
															if input.OnRemove != nil {
																input.OnRemove(origID)
															}
														})
													}),
												)
											})
										}),
									)
								})
							})
						})
					}

					return chrome.Inset(gtx, layout.Inset{Top: 14}, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !canRemove {
									return layout.Dimensions{}
								}
								return chrome.DangerButton(gtx, &c.agent.RemoveButton, "Remove agent", canRemove, func() {
									c.agent.ConfirmDelete = true
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return chrome.Inset(gtx, layout.Inset{Right: 8}, func(gtx layout.Context) layout.Dimensions {
											return chrome.Button(gtx, &c.agent.CancelButton, "Cancel", enabled, func() {
												c.agent.EditorVisible = false
												c.agent.EditorOriginalID = ""
												c.agent.EditorKey = ""
												c.agent.ConfirmDelete = false
												c.ClearAgentProfileEditors()
												gtx.Execute(op.InvalidateCmd{})
											})
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return chrome.PrimaryButton(gtx, &c.agent.SaveButton, "Save agent", canSave, func() {
											origID := c.agent.EditorOriginalID
											agentID := c.agent.IDEditor.Text()
											name := c.agent.NameEditor.Text()
											cmd := c.agent.CommandEditor.Text()
											args := c.agent.ArgsEditor.Text()
											env := c.agent.EnvEditor.Text()
											c.agent.EditorVisible = false
											c.agent.EditorOriginalID = ""
											c.agent.EditorKey = ""
											c.agent.ConfirmDelete = false
											c.ClearAgentProfileEditors()
											if input.OnSave != nil {
												input.OnSave(origID, agentID, name, cmd, args, env)
											}
										})
									}),
								)
							}),
						)
					})
				}),
			)
		})
	})
}

func (c *Component) layoutAgentPresetsBar(gtx layout.Context, enabled bool, chrome Chrome) layout.Dimensions {
	if c.agent.PresetProtonmanButton.Clicked(gtx) {
		c.agent.IDEditor.SetText("protonman")
		c.agent.NameEditor.SetText("Protonman")
		c.agent.CommandEditor.SetText("protonman")
		c.agent.ArgsEditor.SetText("--acp")
		c.agent.EnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if c.agent.PresetOpencodeButton.Clicked(gtx) {
		c.agent.IDEditor.SetText("opencode")
		c.agent.NameEditor.SetText("OpenCode")
		c.agent.CommandEditor.SetText("opencode")
		c.agent.ArgsEditor.SetText("acp")
		c.agent.EnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if c.agent.PresetClineButton.Clicked(gtx) {
		c.agent.IDEditor.SetText("cline")
		c.agent.NameEditor.SetText("Cline")
		c.agent.CommandEditor.SetText("cline")
		c.agent.ArgsEditor.SetText("--acp")
		c.agent.EnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if c.agent.PresetAntigravityButton.Clicked(gtx) {
		c.agent.IDEditor.SetText("antigravity")
		c.agent.NameEditor.SetText("Antigravity")
		c.agent.CommandEditor.SetText("agy")
		c.agent.ArgsEditor.SetText("--acp")
		c.agent.EnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if c.agent.PresetClaudeButton.Clicked(gtx) {
		c.agent.IDEditor.SetText("claude")
		c.agent.NameEditor.SetText("Claude Code")
		c.agent.CommandEditor.SetText("claude")
		c.agent.ArgsEditor.SetText("--acp")
		c.agent.EnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if c.agent.PresetCustomButton.Clicked(gtx) {
		c.ClearAgentProfileEditors()
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "QUICK PRESETS", unit.Sp(11), font.Bold, chrome.Colors.OnSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 4}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.LayoutPresetChip(gtx, &c.agent.PresetProtonmanButton, "Protonman", enabled, chrome)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return c.LayoutPresetChip(gtx, &c.agent.PresetOpencodeButton, "OpenCode", enabled, chrome)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return c.LayoutPresetChip(gtx, &c.agent.PresetClineButton, "Cline", enabled, chrome)
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.LayoutPresetChip(gtx, &c.agent.PresetAntigravityButton, "Antigravity", enabled, chrome)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return c.LayoutPresetChip(gtx, &c.agent.PresetClaudeButton, "Claude Code", enabled, chrome)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return c.LayoutPresetChip(gtx, &c.agent.PresetCustomButton, "Custom", enabled, chrome)
						})
					}),
				)
			})
		}),
	)
}
