//go:build desktop || desktop_gio

package shell

import (
	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	inspectorcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/inspector"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

var (
	reasoningChoices      = []string{"auto", "none", "minimal", "low", "medium", "high", "xhigh", "max"}
	lowConcurrencyChoices = []string{"auto", "on", "off"}
	permissionModeChoices = []string{"ask", "plan", "always-approve"}
)

func (s *Shell) syncRuntimeEditors(state desktopstate.State) {
	session, ok := selectedSession(state)
	if !ok {
		if s.runtimeComponent.Widgets().RuntimeEditorKey != "" {
			s.runtimeComponent.Widgets().RuntimeEditorKey = ""
			s.runtimeComponent.Widgets().RuntimeEditorProvider = ""
			s.runtimeComponent.Widgets().RuntimeEditorModel = ""
			s.runtimeComponent.Widgets().RuntimeProviderEditor.SetText("")
			s.runtimeComponent.Widgets().RuntimeModelEditor.SetText("")
		}
		return
	}
	if session.ID != s.runtimeComponent.Widgets().RuntimeEditorKey {
		s.runtimeComponent.Widgets().RuntimeEditorKey = session.ID
		s.runtimeComponent.Widgets().RuntimeEditorProvider = session.Runtime.Provider
		s.runtimeComponent.Widgets().RuntimeEditorModel = session.Runtime.Model
		s.runtimeComponent.Widgets().RuntimeProviderEditor.SetText(session.Runtime.Provider)
		s.runtimeComponent.Widgets().RuntimeModelEditor.SetText(session.Runtime.Model)
		return
	}
	if session.Runtime.Provider != s.runtimeComponent.Widgets().RuntimeEditorProvider {
		if s.runtimeComponent.Widgets().RuntimeProviderEditor.Text() == s.runtimeComponent.Widgets().RuntimeEditorProvider {
			s.runtimeComponent.Widgets().RuntimeEditorProvider = session.Runtime.Provider
			s.runtimeComponent.Widgets().RuntimeProviderEditor.SetText(session.Runtime.Provider)
		}
	}
	if session.Runtime.Model != s.runtimeComponent.Widgets().RuntimeEditorModel {
		if s.runtimeComponent.Widgets().RuntimeModelEditor.Text() == s.runtimeComponent.Widgets().RuntimeEditorModel {
			s.runtimeComponent.Widgets().RuntimeEditorModel = session.Runtime.Model
			s.runtimeComponent.Widgets().RuntimeModelEditor.SetText(session.Runtime.Model)
		}
	}
}

func (s *Shell) layoutInspector(gtx layout.Context, session desktopstate.SessionState, snapshot controller.Snapshot, compact bool) layout.Dimensions {
	return s.inspectorComponent.Layout(gtx, inspectorcomponent.ViewInput{
		Snapshot: inspectorcomponent.Snapshot{
			ExternalAgent: !protonmanSession(session),
			Connected:     sessionConnection(snapshot, session.AgentID) == controller.ConnectionConnected,
			AgentName:     agentDisplayName(snapshot.AgentProfiles, session.AgentID),
		},
		Chrome: s.inspectorChrome(),
		Panel: func(gtx layout.Context, tab, index int) layout.Dimensions {
			return s.layoutInspectorPanel(gtx, session, snapshot, tab, index)
		},
	}, compact)
}

func (s *Shell) inspectorChrome() inspectorcomponent.Chrome {
	return s.baseChrome()
}

func (s *Shell) layoutInspectorPanel(gtx layout.Context, session desktopstate.SessionState, snapshot controller.Snapshot, tab, index int) layout.Dimensions {
	switch tab {
	case 0:
		if index == 0 {
			return s.layoutGoalPanel(gtx, session)
		}
		if index == 1 {
			return s.layoutTodoPanel(gtx, session)
		}
		return s.layoutRuntimePanel(gtx, session, snapshot)
	case 1:
		return s.layoutMemoryPanel(gtx, session)
	case 2:
		return s.layoutSkillsPanel(gtx, session)
	default:
		return s.layoutGoalPanel(gtx, session)
	}
}

func protonmanSession(session desktopstate.SessionState) bool {
	return session.AgentID == "" || session.AgentID == controller.ProtonmanAgentID
}

func (s *Shell) layoutGoalPanel(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	return s.inspectorComponent.LayoutGoal(gtx, session.Context.Goal, s.inspectorChrome())
}

func (s *Shell) layoutTodoPanel(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	todo := inspectorcomponent.Todo{Revision: session.Context.Todo.Revision, Items: make([]inspectorcomponent.TodoItem, 0, len(session.Context.Todo.Items))}
	for _, item := range session.Context.Todo.Items {
		todo.Items = append(todo.Items, inspectorcomponent.TodoItem{Status: item.Status, Text: item.Text})
	}
	return s.inspectorComponent.LayoutTodo(gtx, todo, s.inspectorChrome())
}

func (s *Shell) layoutMemoryPanel(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	memory := inspectorcomponent.Memory{
		Workspace: convertMemoryEntries(session.Context.Memory.Workspace),
		Global:    convertMemoryEntries(session.Context.Memory.Global),
	}
	return s.inspectorComponent.LayoutMemory(gtx, memory, s.inspectorChrome())
}

func convertMemoryEntries(entries []desktopstate.MemoryEntryState) []inspectorcomponent.MemoryEntry {
	converted := make([]inspectorcomponent.MemoryEntry, 0, len(entries))
	for _, entry := range entries {
		converted = append(converted, inspectorcomponent.MemoryEntry{
			ID: entry.ID, Kind: entry.Kind, Key: entry.Key, Value: entry.Value,
			Confidence: entry.Confidence, UsageCount: entry.UsageCount,
		})
	}
	return converted
}

func (s *Shell) layoutRuntimePanel(gtx layout.Context, session desktopstate.SessionState, snapshot controller.Snapshot) layout.Dimensions {
	busy := controller.SessionBusy(session.Status)
	enabled := sessionConnection(snapshot, session.AgentID) == controller.ConnectionConnected && !busy && !snapshot.RuntimeUpdating
	s.runtimeComponent.Widgets().RuntimeProviderEditor.ReadOnly = !enabled
	s.runtimeComponent.Widgets().RuntimeModelEditor.ReadOnly = !enabled

	return s.inspectorComponent.LayoutRuntime(gtx, inspectorcomponent.RuntimePanelInput{
		ProviderEditor:         &s.runtimeComponent.Widgets().RuntimeProviderEditor,
		ModelEditor:            &s.runtimeComponent.Widgets().RuntimeModelEditor,
		ApplyButton:            &s.runtimeComponent.Widgets().RuntimeApplyButton,
		ReasoningChoices:       reasoningChoices,
		SelectedReasoning:      session.Runtime.Reasoning,
		ReasoningButtons:       s.runtimeComponent.Widgets().ReasoningButtons,
		LowConcurrencyChoices:  lowConcurrencyChoices,
		SelectedLowConcurrency: session.Runtime.LowConcurrency,
		LowConcurrencyButtons:  s.runtimeComponent.Widgets().LowConcurrencyButtons,
		PermissionModeChoices:  permissionModeChoices,
		SelectedPermissionMode: session.Runtime.PermissionMode,
		PermissionModeButtons:  s.runtimeComponent.Widgets().PermissionModeButtons,
		Enabled:                enabled,
		Updating:               snapshot.RuntimeUpdating,
		Chrome:                 s.inspectorChrome(),
		OnSetModel:             s.bind.SetRuntimeModel,
		OnSetReasoning:         s.bind.SetRuntimeReasoning,
		OnSetLowConcurrency:    s.bind.SetRuntimeLow,
		OnSetPermissionMode:    s.bind.SetRuntimePermissionMode,
	})
}

func (s *Shell) layoutPanelTitle(gtx layout.Context, title string) layout.Dimensions {
	return s.layoutLabel(gtx, title, textTitleMedium, font.SemiBold, s.theme.Colors.OnSurface, 2)
}

func (s *Shell) layoutInspectorEditor(gtx layout.Context, label string, editor *widget.Editor, enabled bool) layout.Dimensions {
	textColor := s.theme.Colors.OnSurface
	if !enabled {
		textColor = s.theme.Colors.OnSurfaceVariant
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, s.theme.Colors.OnSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if gtx.Constraints.Max.X > 0 {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
			}
			gtx.Constraints.Min.Y = gtx.Dp(40)
			semantic.EnabledOp(enabled).Add(gtx.Ops)
			semantic.DescriptionOp(label).Add(gtx.Ops)
			dims := s.roundedSurface(gtx, shapeSmall, s.theme.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
				if gtx.Constraints.Max.X > 0 {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
				}
				gtx.Constraints.Min.Y = gtx.Dp(40)
				return uikit.Inset{Top: 9, Bottom: 9, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(s.theme.Material, editor, "")
					ed.TextSize = textBodyMedium
					ed.Color = textColor
					ed.SelectionColor = s.theme.Colors.PrimaryContainer
					return ed.Layout(gtx)
				})
			})
			if dims.Size.X > 0 && dims.Size.Y > 0 {
				borderColor := s.theme.Colors.OutlineVariant
				borderWidth := unit.Dp(1)
				if enabled && gtx.Focused(editor) {
					borderColor = s.theme.Colors.Primary
					borderWidth = unit.Dp(2)
				}
				widget.Border{Color: borderColor, CornerRadius: shapeSmall, Width: borderWidth}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: dims.Size}
				})
			}
			return dims
		}),
	)
}

func (s *Shell) layoutSkillsPanel(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	skills := make([]inspectorcomponent.Skill, 0, len(session.Skills))
	for _, skill := range session.Skills {
		skills = append(skills, inspectorcomponent.Skill{
			Name: skill.Name, Scope: skill.Scope, Description: skill.Description, Active: skill.Active,
			Resources: append([]string(nil), skill.Resources...),
		})
	}
	return s.inspectorComponent.LayoutSkills(gtx, inspectorcomponent.SkillsInput{
		SessionID: session.ID, Skills: skills, Chrome: s.inspectorChrome(), OnToggle: s.bind.ToggleSkill,
	})
}

func compactInspectorText(value string, maxRunes int) string {
	return inspectorcomponent.CompactText(value, maxRunes)
}

func todoMarkerForStatus(status string) string {
	return inspectorcomponent.TodoMarker(status)
}
