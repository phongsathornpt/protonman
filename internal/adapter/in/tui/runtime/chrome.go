package runtime

import (
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func promptPlaceholder(hasRunner bool, mode permission.Mode, planMode bool) string {
	if !hasRunner {
		return "Message or /command…"
	}
	if planMode {
		return "Plan or inspect…"
	}
	switch mode {
	case permission.ModeAlwaysApprove:
		return "Message Protonman…"
	case permission.ModeDeny:
		return "Inspect workspace…"
	default:
		return "Message Protonman…"
	}
}

func (m *bubbleModel) resetTranscript() {
	m.ensureHistoryState().Reset()
	m.conversationViewport.setFollowing(true)
	m.showWelcome = true
	m.refreshTranscriptViewport(true)
}

func (m *bubbleModel) resetConversation() {
	m.ensureHistoryState().Reset()
	m.messages = nil
	m.queue = nil
	m.conversationViewport.setFollowing(true)
	m.showWelcome = true
	if m.skills != nil {
		m.skills.ResetActivated()
	}
	m.refreshTranscriptViewport(true)
} // resetConversation clears both the visible transcript and provider history.
// Keeping this separate from resetTranscript makes Ctrl+L a safe display-only
// action while /new has the explicit semantics users expect from its name.

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func shortcutHelp(binding key.Binding) string {
	help := binding.Help()
	return strings.TrimSpace(help.Key + " " + help.Desc)
}
func (m *bubbleModel) promptView() string {
	if m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return ""
	}
	prompt := m.panes.bottom.prompt().View()
	meta := m.promptMetadataView()
	if meta == "" {
		return prompt
	}
	return meta + "\n" + prompt
}

func (m *bubbleModel) promptMetadataView() string {
	if m == nil {
		return ""
	}
	available := maxInt(1, m.layout.width-2)
	mode := m.promptModeLabel()
	parts := promptMetadataParts(m.activeModel, m.agentProfile, m.workDir)
	full := append(append([]string(nil), parts...), mode)
	if rendered := strings.Join(full, " · "); ansi.StringWidth(rendered) <= available {
		return mutedStyle.Render(rendered)
	}
	compact := compactPromptMetadataParts(m.activeModel, m.workDir, mode, available)
	return mutedStyle.Render(strings.Join(compact, " · "))
}

func promptMetadataParts(model, agent, workDir string) []string {
	parts := make([]string, 0, 3)
	if model = strings.TrimSpace(model); model != "" {
		parts = append(parts, model)
	}
	if agent = strings.TrimSpace(agent); agent != "" {
		parts = append(parts, agent)
	}
	if workspace := formatWorkspaceDisplay(workDir); workspace != "" {
		parts = append(parts, workspace)
	}
	return parts
}

func compactPromptMetadataParts(model, workDir, mode string, available int) []string {
	model = strings.TrimSpace(model)
	if slash := strings.LastIndex(model, "/"); slash >= 0 && slash+1 < len(model) {
		model = model[slash+1:]
	}
	workspace := formatWorkspaceDisplay(workDir)
	if workspace != "" {
		workspace = filepath.Base(workspace)
	}
	parts := make([]string, 0, 3)
	for _, value := range []string{model, workspace, mode} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	for ansi.StringWidth(strings.Join(parts, " · ")) > available && len(parts) > 1 {
		// Preserve the mode and workspace identity; trim the model first.
		budget := available - ansi.StringWidth(strings.Join(parts[1:], " · ")) - 3
		if budget > 1 {
			parts[0] = truncateWithEllipsis(parts[0], budget)
			break
		}
		parts = parts[1:]
	}
	if rendered := strings.Join(parts, " · "); ansi.StringWidth(rendered) > available {
		return []string{truncateWithEllipsis(mode, available)}
	}
	return parts
}

func (m *bubbleModel) promptModeLabel() string {
	if m.planMode {
		return "plan"
	}
	if m.service == nil {
		return "ask"
	}
	switch m.service.Mode() {
	case permission.ModeAlwaysApprove:
		return "auto"
	case permission.ModeDeny:
		return "deny"
	default:
		return "ask"
	}
}
