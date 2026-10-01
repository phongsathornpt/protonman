//go:build desktop || desktop_gio

package inspector

import (
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Skill struct {
	Name, Scope, Description string
	Active                   bool
	Resources                []string
}

type SkillsInput struct {
	SessionID string
	Skills    []Skill
	Chrome    Chrome
	OnToggle  func(sessionID, skillName string)
}

// LayoutSkills owns filtering and toggle widgets for the inspector skills tab.
func (c *Component) LayoutSkills(gtx layout.Context, input SkillsInput) layout.Dimensions {
	chrome := input.Chrome
	active := 0
	for _, skill := range input.Skills {
		if skill.Active {
			active++
		}
	}
	filter := strings.ToLower(strings.TrimSpace(c.skillSearch.Text()))
	filtered := make([]Skill, 0, len(input.Skills))
	for _, skill := range input.Skills {
		if filter == "" || strings.Contains(strings.ToLower(skill.Name), filter) || strings.Contains(strings.ToLower(skill.Description), filter) {
			filtered = append(filtered, skill)
		}
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Label(gtx, "Agent Skills", unit.Sp(16), font.SemiBold, chrome.Colors.OnSurface, 2)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, fmt.Sprintf("%d/%d active", active, len(input.Skills)), unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
						})
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(28)
				return chrome.BorderSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerHighest, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
						c.skillSearch.SingleLine = true
						c.skillSearch.MaxLen = 128
						ed := material.Editor(chrome.Material, &c.skillSearch, "Filter skills…")
						ed.TextSize, ed.Color, ed.HintColor = unit.Sp(12), chrome.Colors.OnSurface, chrome.Colors.OnSurfaceVariant
						return ed.Layout(gtx)
					})
				})
			})
		}),
	}
	if len(input.Skills) == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.skillEmpty(gtx, "No agent skills discovered. Add skills to ~/.protonman/skills/ or .protonman/skills/ in your workspace.", chrome, 3)
		}))
	} else if len(filtered) == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.skillEmpty(gtx, "No skills match the current filter.", chrome, 2)
		}))
	} else {
		for _, skill := range filtered {
			skill := skill
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.layoutSkillCard(gtx, input, skill) }))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (c *Component) skillEmpty(gtx layout.Context, text string, chrome Chrome, lines int) layout.Dimensions {
	return chrome.Inset(gtx, layout.Inset{Top: 8}, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(8), chrome.Colors.SurfaceContainerLowest, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, text, unit.Sp(14), font.Normal, chrome.Colors.OnSurfaceVariant, lines)
			})
		})
	})
}

func (c *Component) layoutSkillCard(gtx layout.Context, input SkillsInput, skill Skill) layout.Dimensions {
	if c.skillButtons == nil {
		c.skillButtons = make(map[string]*widget.Clickable)
	}
	button := c.skillButtons[skill.Name]
	if button == nil {
		button = new(widget.Clickable)
		c.skillButtons[skill.Name] = button
	}
	if button.Clicked(gtx) && input.OnToggle != nil {
		input.OnToggle(input.SessionID, skill.Name)
	}
	toggleLabel, toggleBg, toggleFg := "Enable", input.Chrome.Colors.SurfaceContainerHigh, input.Chrome.Colors.OnSurfaceVariant
	if skill.Active {
		toggleLabel, toggleBg, toggleFg = "Active", input.Chrome.Colors.PrimaryContainer, input.Chrome.Colors.OnPrimaryContainer
	}
	chrome := input.Chrome
	return chrome.Inset(gtx, layout.Inset{Top: 3, Bottom: 3}, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLowest, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, skill.Name, unit.Sp(14), font.SemiBold, chrome.Colors.OnSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Inset(gtx, layout.Inset{Left: 6}, func(gtx layout.Context) layout.Dimensions {
									return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
										return chrome.Inset(gtx, layout.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
											return chrome.Label(gtx, strings.ToUpper(skill.Scope), unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
										})
									})
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return chrome.RoundedSurface(gtx, unit.Dp(4), toggleBg, func(gtx layout.Context) layout.Dimensions {
										return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
											return chrome.Label(gtx, toggleLabel, unit.Sp(11), font.SemiBold, toggleFg, 1)
										})
									})
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						description := strings.TrimSpace(skill.Description)
						if description == "" {
							description = "No description provided."
						}
						return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, description, unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 3)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if len(skill.Resources) == 0 {
							return layout.Dimensions{}
						}
						return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, fmt.Sprintf("%d bundled resources", len(skill.Resources)), unit.Sp(11), font.Normal, chrome.Colors.Outline, 1)
						})
					}),
				)
			})
		})
	})
}
