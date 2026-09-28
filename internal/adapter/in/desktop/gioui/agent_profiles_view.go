//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"image/color"
	"log"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/app"
)

func activeAgentDisplayName(snapshot controllerSnapshot) string {
	return agentDisplayName(snapshot.AgentProfiles, snapshot.ActiveAgentID)
}

func agentDisplayName(profiles []app.ACPAgentProfile, agentID string) string {
	agentID = strings.TrimSpace(agentID)
	for _, profile := range profiles {
		if profile.ID == agentID {
			return profile.DisplayName
		}
	}
	if agentID == "" {
		return "Not configured"
	}
	return agentID
}

func (s *shell) syncAgentProfileEditors(snapshot controllerSnapshot) {
	if s.syncRevision != 0 && s.agentSyncRevisionSet && s.agentSyncRevision == s.syncRevision {
		return
	}
	live := s.agentProfileLive
	if live == nil {
		live = make(map[string]struct{}, len(snapshot.AgentProfiles))
		s.agentProfileLive = live
	}
	clear(live)
	for _, profile := range snapshot.AgentProfiles {
		live[profile.ID] = struct{}{}
		if s.agentProfileButtons[profile.ID] == nil {
			s.agentProfileButtons[profile.ID] = new(widget.Clickable)
		}
		if s.agentChoiceButtons[profile.ID] == nil {
			s.agentChoiceButtons[profile.ID] = new(widget.Clickable)
		}
	}
	for agentID := range s.agentProfileButtons {
		if _, ok := live[agentID]; !ok {
			delete(s.agentProfileButtons, agentID)
		}
	}
	for agentID := range s.agentChoiceButtons {
		if _, ok := live[agentID]; !ok {
			delete(s.agentChoiceButtons, agentID)
		}
	}

	if s.agentEditorOriginalID != "" {
		if _, ok := live[s.agentEditorOriginalID]; !ok {
			s.agentEditorOriginalID = ""
			s.agentEditorKey = ""
			s.agentEditorVisible = false
			s.clearAgentProfileEditors()
		}
	}
	if !s.agentEditorVisible || s.agentEditorOriginalID == "" || s.agentEditorOriginalID == s.agentEditorKey {
		if s.syncRevision != 0 {
			s.agentSyncRevision = s.syncRevision
			s.agentSyncRevisionSet = true
		}
		return
	}
	for _, profile := range snapshot.AgentProfiles {
		if profile.ID != s.agentEditorOriginalID {
			continue
		}
		s.agentEditorKey = profile.ID
		s.agentIDEditor.SetText(profile.ID)
		s.agentNameEditor.SetText(profile.DisplayName)
		s.agentCommandEditor.SetText(profile.Command)
		s.agentArgsEditor.SetText(mcpJSONList(profile.Args))
		s.agentEnvEditor.SetText(mcpJSONList(profile.Env))
		if s.syncRevision != 0 {
			s.agentSyncRevision = s.syncRevision
			s.agentSyncRevisionSet = true
		}
		return
	}
	if s.syncRevision != 0 {
		s.agentSyncRevision = s.syncRevision
		s.agentSyncRevisionSet = true
	}
}

func (s *shell) layoutAgentSelectorBar(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(56)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Spacer{}.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(320)
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(460))
			return s.roundedSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 6, Bottom: 6, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Choose agent", textBodyMedium, font.Medium, s.theme.onSurfaceVariant, 1)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Spacer{}.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.agentSelectorList.Layout(gtx, len(snapshot.AgentProfiles), func(gtx layout.Context, index int) layout.Dimensions {
								profile := snapshot.AgentProfiles[index]
								return desktopUniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutAgentChoiceButton(gtx, profile, profile.ID == snapshot.ActiveAgentID, snapshot.AgentConnections[profile.ID])
								})
							})
						}),
					)
				})
			})
		}),
	)
}

func (s *shell) layoutAgentChoiceButton(gtx layout.Context, profile app.ACPAgentProfile, selected bool, phase connectionPhase) layout.Dimensions {
	button := s.agentChoiceButtons[profile.ID]
	if button.Clicked(gtx) {
		s.onSelectAgent(profile.ID)
		s.agentSelectorVisible = false
	}
	background := s.theme.surface
	foreground := s.theme.onSurface
	if selected {
		background = s.theme.primaryContainer
		foreground = s.theme.onPrimaryContainer
	} else if button.Hovered() {
		background = s.theme.primaryContainer
		foreground = s.theme.onPrimaryContainer
	}
	gtx.Constraints.Min.X = gtx.Dp(148)
	gtx.Constraints.Min.Y = gtx.Dp(44)
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(44)
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		semantic.DescriptionOp("Select agent " + profile.DisplayName + ", " + connectionLabel(phase)).Add(gtx.Ops)
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, profile.DisplayName+" · "+connectionChipLabel(phase), textLabelMedium, font.SemiBold, foreground, 1)
			})
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func connectionLabel(phase connectionPhase) string {
	switch phase {
	case connectionConnected:
		return "connected"
	case connectionReconnecting:
		return "reconnecting"
	default:
		return "connecting"
	}
}

func connectionChipLabel(phase connectionPhase) string {
	switch phase {
	case connectionConnected:
		return "ready"
	case connectionReconnecting:
		return "retrying"
	default:
		return "starting"
	}
}

func sessionConnection(snapshot controllerSnapshot, agentID string) connectionPhase {
	if strings.TrimSpace(agentID) == "" {
		agentID = controllerAgentID
	}
	if phase, ok := snapshot.AgentConnections[agentID]; ok {
		return phase
	}
	return snapshot.Connection
}

func anyAgentConnected(snapshot controllerSnapshot) bool {
	for _, phase := range snapshot.AgentConnections {
		if phase == connectionConnected {
			return true
		}
	}
	return snapshot.Connection == connectionConnected
}

func connectionStatusInfo(phase connectionPhase) (color.NRGBA, string) {
	switch phase {
	case connectionConnected:
		return color.NRGBA{R: 52, G: 199, B: 89, A: 255}, "Ready"
	case connectionConnecting:
		return color.NRGBA{R: 255, G: 159, B: 10, A: 255}, "Starting"
	case connectionReconnecting:
		return color.NRGBA{R: 255, G: 159, B: 10, A: 255}, "Retrying"
	default:
		return color.NRGBA{R: 142, G: 142, B: 147, A: 255}, "Offline"
	}
}

func (s *shell) layoutStatusDot(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	size := gtx.Dp(8)
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	paint.FillShape(gtx.Ops, c, clip.Ellipse{Max: image.Pt(size, size)}.Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(size, size)}
}

func (s *shell) layoutAgentProfilesPanel(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	profiles := snapshot.AgentProfiles
	enabled := !snapshot.AgentUpdating
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPanelTitle(gtx, "ACP agents")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			summary := "1 supervised daemon process"
			if len(profiles) != 1 {
				summary = strconv.Itoa(len(profiles)) + " supervised daemon processes · Select an agent card to edit"
			}
			return desktopInset{Top: 2, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, summary, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 2)
			})
		}),
	}

	for _, profile := range profiles {
		profile := profile
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutAgentProfileRow(gtx, profile, profile.ID == s.agentEditorOriginalID && s.agentEditorVisible, enabled, snapshot)
		}))
	}

	if enabled && s.agentFormToggleButton.Clicked(gtx) {
		s.agentEditorVisible = true
		s.agentEditorOriginalID = ""
		s.agentEditorKey = ""
		s.agentConfirmDelete = false
		s.clearAgentProfileEditors()
		log.Printf("[UI] + Add ACP Agent clicked, agentEditorVisible is now true")
		gtx.Execute(op.InvalidateCmd{})
	}

	if !s.agentEditorVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.agentFormToggleButton, "+ Add ACP Agent", enabled, nil)
			})
		}))
	}

	if s.agentEditorVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 10, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutAgentConfigurationCard(gtx, snapshot, profiles, enabled)
			})
		}))
	}

	status := "Daemon changes apply upon restarting Protonman Desktop. Environment values are read at launch and never stored."
	statusColor := s.theme.onSurfaceVariant
	if snapshot.AgentError != "" {
		status = "ACP settings unavailable · " + compactInspectorText(snapshot.AgentError, 240)
		statusColor = s.theme.onErrorContainer
	} else if snapshot.AgentConfigOverridden {
		status = "PROTONMAN_ACP_AGENTS_JSON is active. Remove the environment override to apply changes."
	} else if snapshot.AgentUpdating {
		status = "Saving ACP agent settings…"
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, status, textLabelSmall, font.Normal, statusColor, 4)
		})
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutAgentProfileRow(gtx layout.Context, profile app.ACPAgentProfile, selected, enabled bool, snapshot controllerSnapshot) layout.Dimensions {
	button := s.agentProfileButtons[profile.ID]
	if button == nil {
		button = new(widget.Clickable)
		s.agentProfileButtons[profile.ID] = button
	}
	rowContext := gtx
	if !enabled {
		rowContext = gtx.Disabled()
	}
	if enabled && button.Clicked(rowContext) {
		s.agentEditorOriginalID = profile.ID
		s.agentEditorKey = ""
		s.agentEditorVisible = true
		s.agentConfirmDelete = false
		gtx.Execute(op.InvalidateCmd{})
	}
	gtx.Constraints.Min.Y = gtx.Dp(52)
	bg := s.theme.surface
	borderColor := s.theme.outlineVariant
	borderWidth := 1
	if selected {
		bg = s.theme.surfaceContainerHigh
		borderColor = s.theme.primary
		borderWidth = 2
	} else if button.Hovered() {
		bg = s.theme.surfaceContainerHigh
	}

	phase := sessionConnection(snapshot, profile.ID)
	dotColor, statusText := connectionStatusInfo(phase)
	isActive := profile.ID == snapshot.ActiveAgentID

	cmdText := profile.Command
	if len(profile.Args) > 0 {
		cmdText += " " + strings.Join(profile.Args, " ")
	}

	dims := button.Layout(rowContext, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.roundedBorderSurface(gtx, shapeMedium, bg, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// Header row: Status Dot + Name + ID + Active Badge + Status text + Edit hint
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutStatusDot(gtx, dotColor)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, profile.DisplayName, textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, profile.ID, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
											})
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if !isActive {
										return layout.Dimensions{}
									}
									return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.primaryContainer, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, "ACTIVE", textLabelSmall, font.Bold, s.theme.onPrimaryContainer, 1)
											})
										})
									})
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{}.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, statusText, textLabelSmall, font.Medium, dotColor, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Top: 2, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "Edit", textLabelSmall, font.SemiBold, s.theme.primary, 1)
										})
									})
								}),
							)
						}),
						// Command preview pill
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLowest, func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, compactInspectorText(cmdText, 64), textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
									})
								})
							})
						}),
						// Status detail / error pill when not connected and an error message is available
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if phase == connectionConnected || snapshot.AgentStatuses == nil {
								return layout.Dimensions{}
							}
							errMsg := snapshot.AgentStatuses[profile.ID]
							if errMsg == "" || errMsg == "Starting "+profile.DisplayName+"…" || errMsg == "Offline" || errMsg == "Connected" {
								return layout.Dimensions{}
							}
							return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, s.theme.errorContainer, func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, compactInspectorText(errMsg, 80), textLabelSmall, font.Normal, s.theme.onErrorContainer, 1)
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
		widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *shell) layoutAgentConfigurationCard(gtx layout.Context, snapshot controllerSnapshot, profiles []app.ACPAgentProfile, enabled bool) layout.Dimensions {
	isEditing := s.agentEditorOriginalID != ""
	title := "Add New ACP Agent"
	if isEditing {
		title = "Edit Agent: " + s.agentNameEditor.Text()
		if strings.TrimSpace(title) == "Edit Agent:" {
			title = "Edit Agent: " + s.agentEditorOriginalID
		}
	}
	log.Printf("[UI] rendering layoutAgentConfigurationCard: title=%q", title)

	canRemove := enabled && isEditing && len(profiles) > 1
	canSave := enabled && strings.TrimSpace(s.agentIDEditor.Text()) != "" && strings.TrimSpace(s.agentCommandEditor.Text()) != ""

	if gtx.Constraints.Max.X > 0 {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
	}
	return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		if gtx.Constraints.Max.X > 0 {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
		}
		return desktopInset{Top: 16, Bottom: 16, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Header
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, title, textTitleMedium, font.Bold, s.theme.onSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if s.agentCloseButton.Clicked(gtx) {
								log.Printf("[UI] agentCloseButton clicked")
								s.agentEditorVisible = false
								s.agentEditorOriginalID = ""
								s.agentEditorKey = ""
								s.agentConfirmDelete = false
								s.clearAgentProfileEditors()
								gtx.Execute(op.InvalidateCmd{})
							}
							return s.layoutMiniIconButton(gtx, &s.agentCloseButton, "✕", s.theme.onSurfaceVariant)
						}),
					)
				}),
				// Presets bar (only shown when adding a new agent)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if isEditing {
						return layout.Dimensions{}
					}
					return desktopInset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutAgentPresetsBar(gtx, enabled)
					})
				}),
				// Form editors
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 8, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutAgentProfileEditor(gtx, "Agent ID", &s.agentIDEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutAgentProfileEditor(gtx, "Display name", &s.agentNameEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutAgentProfileEditor(gtx, "Command", &s.agentCommandEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutAgentProfileEditor(gtx, "Arguments (flags or space-separated, e.g. --acp)", &s.agentArgsEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutAgentProfileEditor(gtx, "Environment keys (space or comma-separated)", &s.agentEnvEditor, enabled)
					})
				}),
				// Actions / Delete Confirmation
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.agentConfirmDelete {
						return desktopInset{Top: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedBorderSurface(gtx, shapeSmall, s.theme.errorContainer, s.theme.errorContainer, 1, func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "Delete this agent profile? This cannot be undone.", textBodySmall, font.SemiBold, s.theme.onErrorContainer, 2)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return s.layoutButton(gtx, &s.agentCancelDeleteBtn, "Cancel", enabled, func() {
															s.agentConfirmDelete = false
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return s.layoutDangerButton(gtx, &s.agentConfirmDeleteBtn, "Confirm Delete", enabled, func() {
															origID := s.agentEditorOriginalID
															s.agentEditorVisible = false
															s.agentEditorOriginalID = ""
															s.agentEditorKey = ""
															s.agentConfirmDelete = false
															s.clearAgentProfileEditors()
															s.onRemoveAgentProfile(origID)
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

					return desktopInset{Top: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !canRemove {
									return layout.Dimensions{}
								}
								return s.layoutDangerButton(gtx, &s.agentRemoveButton, "Remove agent", canRemove, func() {
									s.agentConfirmDelete = true
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutButton(gtx, &s.agentCancelButton, "Cancel", enabled, func() {
												s.agentEditorVisible = false
												s.agentEditorOriginalID = ""
												s.agentEditorKey = ""
												s.agentConfirmDelete = false
												s.clearAgentProfileEditors()
												gtx.Execute(op.InvalidateCmd{})
											})
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutPrimaryButton(gtx, &s.agentSaveButton, "Save agent", canSave, func() {
											origID := s.agentEditorOriginalID
											agentID := s.agentIDEditor.Text()
											name := s.agentNameEditor.Text()
											cmd := s.agentCommandEditor.Text()
											args := s.agentArgsEditor.Text()
											env := s.agentEnvEditor.Text()
											s.agentEditorVisible = false
											s.agentEditorOriginalID = ""
											s.agentEditorKey = ""
											s.agentConfirmDelete = false
											s.clearAgentProfileEditors()
											s.onSaveAgentProfile(origID, agentID, name, cmd, args, env)
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

func (s *shell) layoutAgentPresetsBar(gtx layout.Context, enabled bool) layout.Dimensions {
	if s.agentPresetProtonmanBtn.Clicked(gtx) {
		log.Printf("[UI] preset clicked: Protonman")
		s.agentIDEditor.SetText("protonman")
		s.agentNameEditor.SetText("Protonman")
		s.agentCommandEditor.SetText("protonman")
		s.agentArgsEditor.SetText("--acp")
		s.agentEnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if s.agentPresetOpencodeBtn.Clicked(gtx) {
		log.Printf("[UI] preset clicked: OpenCode")
		s.agentIDEditor.SetText("opencode")
		s.agentNameEditor.SetText("OpenCode")
		s.agentCommandEditor.SetText("opencode")
		s.agentArgsEditor.SetText("acp")
		s.agentEnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if s.agentPresetClineBtn.Clicked(gtx) {
		log.Printf("[UI] preset clicked: Cline")
		s.agentIDEditor.SetText("cline")
		s.agentNameEditor.SetText("Cline")
		s.agentCommandEditor.SetText("cline")
		s.agentArgsEditor.SetText("--acp")
		s.agentEnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if s.agentPresetAntigravityBtn.Clicked(gtx) {
		log.Printf("[UI] preset clicked: Antigravity")
		s.agentIDEditor.SetText("antigravity")
		s.agentNameEditor.SetText("Antigravity")
		s.agentCommandEditor.SetText("agy")
		s.agentArgsEditor.SetText("--acp")
		s.agentEnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if s.agentPresetClaudeBtn.Clicked(gtx) {
		log.Printf("[UI] preset clicked: Claude Code")
		s.agentIDEditor.SetText("claude")
		s.agentNameEditor.SetText("Claude Code")
		s.agentCommandEditor.SetText("claude")
		s.agentArgsEditor.SetText("--acp")
		s.agentEnvEditor.SetText("")
		gtx.Execute(op.InvalidateCmd{})
	}
	if s.agentPresetCustomBtn.Clicked(gtx) {
		log.Printf("[UI] preset clicked: Custom")
		s.clearAgentProfileEditors()
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "QUICK PRESETS", textLabelSmall, font.Bold, s.theme.onSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutPresetChip(gtx, &s.agentPresetProtonmanBtn, "Protonman", enabled)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutPresetChip(gtx, &s.agentPresetOpencodeBtn, "OpenCode", enabled)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutPresetChip(gtx, &s.agentPresetClineBtn, "Cline", enabled)
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 2, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutPresetChip(gtx, &s.agentPresetAntigravityBtn, "Antigravity", enabled)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutPresetChip(gtx, &s.agentPresetClaudeBtn, "Claude Code", enabled)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutPresetChip(gtx, &s.agentPresetCustomBtn, "Custom", enabled)
						})
					}),
				)
			})
		}),
	)
}

func (s *shell) layoutPresetChip(gtx layout.Context, btn *widget.Clickable, label string, enabled bool) layout.Dimensions {
	bg := s.theme.surfaceContainerLow
	fg := s.theme.onSurface
	borderColor := s.theme.outlineVariant
	if btn.Hovered() {
		bg = s.theme.surfaceContainerHigh
		borderColor = s.theme.primary
	}
	ctx := gtx
	if !enabled {
		ctx = gtx.Disabled()
	}
	return btn.Layout(ctx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeSmall, bg, borderColor, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelSmall, font.SemiBold, fg, 1)
			})
		})
	})
}

func (s *shell) layoutAgentProfileEditor(gtx layout.Context, label string, editor *widget.Editor, enabled bool) layout.Dimensions {
	return s.layoutInspectorEditor(gtx, label, editor, enabled)
}

func (s *shell) clearAgentProfileEditors() {
	s.agentIDEditor.SetText("")
	s.agentNameEditor.SetText("")
	s.agentCommandEditor.SetText("")
	s.agentArgsEditor.SetText("")
	s.agentEnvEditor.SetText("")
	s.agentConfirmDelete = false
}
