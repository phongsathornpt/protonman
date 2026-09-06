package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/modelprofile"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func (m *bubbleModel) handleReasoningCommand(argument string) tea.Cmd {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		m.showReasoningState()
		m.refreshViewport()
		return nil
	}

	effort, err := sdk.ParseReasoningEffort(argument)
	if err != nil {
		m.appendError("invalid reasoning effort: use auto, none, low, medium, high, xhigh, or max")
		m.refreshViewport()
		return nil
	}
	if effort != sdk.ReasoningDefault {
		profile := m.activeResolvedModelProfile()
		if _, err := profile.ResolveExplicitReasoning(effort); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
	}

	m.reasoningEffort = effort
	if m.coordinator != nil {
		m.coordinator.SetReasoningEffort(effort)
	}
	m.reconfigureRunner()
	m.appendLine(successStyle.Render("Reasoning effort set to " + reasoningEffortLabel(effort) + "."))
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) showReasoningState() {
	profile := m.activeResolvedModelProfile()
	m.appendLine(brandStyle.Render("Reasoning override: ") + reasoningEffortLabel(m.reasoningEffort))
	if profileName := strings.TrimSpace(profile.ProfileName); profileName != "" {
		m.appendLine(mutedStyle.Render("Model profile: " + profileName))
	}
	if len(profile.Reasoning.Levels) > 0 {
		levels := make([]string, 0, len(profile.Reasoning.Levels))
		for _, level := range profile.Reasoning.Levels {
			levels = append(levels, string(level))
		}
		m.appendLine(mutedStyle.Render("Supported reasoning: " + strings.Join(levels, ", ")))
	}
	if profile.Reasoning.Default != sdk.ReasoningDefault {
		m.appendLine(mutedStyle.Render("Model default: " + string(profile.Reasoning.Default)))
	}
	if m.reasoningEffort == sdk.ReasoningDefault {
		if agentProfile, err := agent.ParseProfile(m.agentProfile); err == nil {
			if spec, ok := agent.SpecForProfile(agentProfile); ok && spec.Reasoning != sdk.ReasoningDefault {
				if effective, ok := profile.ResolveProfileReasoning(spec.Reasoning); ok {
					m.appendLine(mutedStyle.Render(fmt.Sprintf("Agent profile preference: %s → %s", spec.Reasoning, effective)))
				} else {
					m.appendLine(mutedStyle.Render("Agent profile preference: " + string(spec.Reasoning)))
				}
			}
		}
	}
}

func (m *bubbleModel) activeResolvedModelProfile() modelprofile.Resolved {
	var remote *model.RemoteModel
	if candidate, ok := m.activeRemoteModel(); ok {
		copy := candidate
		remote = &copy
	}
	return model.ResolveModelProfile(m.activeProvider, m.activeModel, remote)
}

func reasoningEffortLabel(effort sdk.ReasoningEffort) string {
	if effort == sdk.ReasoningDefault {
		return "auto"
	}
	return string(effort)
}

func remoteModelReasoningSummary(providerName string, md model.RemoteModel, includeDefault bool) string {
	profile := model.ResolveModelProfile(providerName, md.ID, &md)
	supported, known := profile.Reasoning.Support.Bool()
	if !known || !supported {
		return ""
	}
	if len(profile.Reasoning.Levels) == 0 {
		return "reasoning"
	}
	levels := make([]string, 0, len(profile.Reasoning.Levels))
	for _, level := range profile.Reasoning.Levels {
		levels = append(levels, string(level))
	}
	summary := "reasoning " + strings.Join(levels, "/")
	if includeDefault && profile.Reasoning.Default != sdk.ReasoningDefault {
		summary += " (default " + string(profile.Reasoning.Default) + ")"
	}
	return summary
}
