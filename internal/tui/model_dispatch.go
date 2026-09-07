package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/app"
	"github.com/projectTHORN/proton/internal/appdirs"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

const (
	maxQueuedPrompts     = 32
	maxQueuePreviewRunes = 160
)

func (m *bubbleModel) submit() tea.Cmd {
	m.syncLegacyToComponents()
	prompt := m.bottom.prompt()
	line := strings.TrimSpace(prompt.Value())
	if m.bottom.bashMode() {
		if line == "" {
			prompt.Reset()
			m.bottom.remove(slashViewID)
			m.setBashMode(false)
			return nil
		}
		if m.busy || m.hasPermissionView() {
			if !m.enqueuePrompt("!" + line) {
				return nil
			}
			prompt.Reset()
			m.bottom.remove(slashViewID)
			m.refreshViewport()
			return nil
		}
		prompt.Reset()
		m.bottom.remove(slashViewID)
		m.setBashMode(false)
		return m.dispatchBang(line)
	}
	if line == "" {
		return nil
	}
	if m.busy || m.hasPermissionView() {
		if !m.enqueuePrompt(line) {
			return nil
		}
		prompt.Reset()
		m.bottom.remove(slashViewID)
		m.refreshViewport()
		return nil
	}
	prompt.Reset()
	m.bottom.remove(slashViewID)
	return m.dispatch(line)
}

func (m *bubbleModel) enqueuePrompt(line string) bool {
	if len(m.queue) >= maxQueuedPrompts {
		m.appendMuted(fmt.Sprintf("queue full (%d); finish or cancel the active turn before adding more", maxQueuedPrompts))
		m.refreshViewport()
		return false
	}
	m.queue = append(m.queue, line)
	m.appendMuted(fmt.Sprintf("queued (%d): %s", len(m.queue), queuePreview(line)))
	return true
}

func queuePreview(line string) string {
	runes := []rune(strings.TrimSpace(line))
	if len(runes) <= maxQueuePreviewRunes {
		return string(runes)
	}
	return string(runes[:maxQueuePreviewRunes-1]) + "…"
}

func (m *bubbleModel) drainQueue() tea.Cmd {
	if m.busy || m.hasPermissionView() || len(m.queue) == 0 {
		return nil
	}
	line := m.queue[0]
	m.queue = m.queue[1:]
	if strings.HasPrefix(line, "!") && !isCommandLine(line) {
		return m.dispatchBang(strings.TrimPrefix(line, "!"))
	}
	return m.dispatch(line)
}

func (m *bubbleModel) dispatch(line string) tea.Cmd {
	m.bottom.recordHistory(line)
	if isCommandLine(line) {
		name, _, _ := splitCommand(line)
		if name != "clear" && name != "new" && name != "quit" && name != "exit" {
			m.appendUser(line)
		}
		return m.executeCommand(line)
	}
	m.appendUser(line)
	return m.startTurn(line)
}

func (m *bubbleModel) dispatchBang(command string) tea.Cmd {
	slog.DebugContext(m.ctx, "tui direct bash submitted", "command_bytes", len(command))
	m.bottom.recordHistory("!" + command)
	m.appendUser("!" + command)
	return m.startBash(command)
}

func (m *bubbleModel) startTool(call tool.Call) tea.Cmd {
	slog.DebugContext(m.ctx, "tui direct tool started",
		"call_id", call.ID,
		"tool_name", call.Name,
		"argument_bytes", len(call.Arguments),
	)
	m.messages = append(m.messages, model.Message{
		Role: model.RoleAssistant,
		ToolCalls: []model.ToolCall{{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: append([]byte(nil), call.Arguments...),
		}},
	})
	m.busy = true
	m.busyStarted = time.Now()
	m.activity = "running " + call.Name
	m.appendToolCall(call)
	m.historyState.SetSpinnerFrame(m.spinner.View())
	m.relayout()

	ctx, cancel := context.WithCancel(m.ctx)
	m.turnCancel = cancel
	return func() tea.Msg {
		defer cancel()
		startedAt := time.Now()
		result, callErr := m.service.Call(ctx, call)
		attrs := []any{
			"call_id", call.ID,
			"tool_name", call.Name,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"success", callErr == nil,
			"error_type", errorType(callErr),
		}
		if result.Failure != nil {
			attrs = append(attrs, "error_code", result.Failure.Code)
		}
		slog.DebugContext(ctx, "tui direct tool worker returned", attrs...)
		return toolResultMsg{call: call, result: result, err: callErr}
	}
}

func (m *bubbleModel) appendModelToolResult(call tool.Call, result tool.Result) {
	if result.CallID == "" {
		result.CallID = call.ID
	}
	if result.ToolName == "" {
		result.ToolName = call.Name
	}
	content, err := json.Marshal(result)
	if err != nil {
		content = []byte(fmt.Sprintf(`{"call_id":%q,"tool_name":%q,"error":{"code":"execution_error","message":%q}}`, call.ID, call.Name, err.Error()))
	}
	m.messages = append(m.messages, model.Message{
		Role:       model.RoleTool,
		Content:    string(content),
		ToolCallID: result.CallID,
		ToolName:   result.ToolName,
	})
}

func (m *bubbleModel) reconfigureRunner() {
	if m.activeModel == "" || m.service == nil {
		return
	}
	provName := m.activeProvider
	if provName == "" {
		provName = model.DefaultProtonmanName
	}
	prov, ok := m.providers[strings.ToLower(provName)]
	hasValidAuth := ok && model.ProviderHasUsableAuth(provName, prov.BaseURL, prov.APIKey)
	if !hasValidAuth {
		dirs, resolveErr := appdirs.Resolve("")
		if resolveErr != nil {
			return
		}
		loaded, err := config.Load(m.ctx, config.Options{HomeDir: dirs.Home, WorkDir: m.workDir})
		if err == nil {
			if m.providers == nil {
				m.providers = make(map[string]config.ProviderConfig)
			}
			for key, value := range loaded.Providers {
				m.providers[key] = value
			}
			prov, ok = m.providers[strings.ToLower(provName)]
			hasValidAuth = ok && model.ProviderHasUsableAuth(provName, prov.BaseURL, prov.APIKey)
		}
	}
	if !hasValidAuth {
		return
	}
	sessID := m.sessionID
	if sessID == "" && m.workDir != "" {
		sessID = "workspace-" + m.workDir
	}
	var remote *model.RemoteModel
	if resolved, ok := m.activeRemoteModel(); ok {
		remote = &resolved
	}
	conversation, err := app.BuildConversation(m.service, m.skills, m.coordinator, app.ConversationSpec{
		ProviderName:    provName,
		ProviderType:    prov.Type,
		BaseURL:         prov.BaseURL,
		APIKey:          prov.APIKey,
		ModelID:         m.activeModel,
		SessionID:       sessID,
		Workspace:       m.workDir,
		AgentProfile:    m.agentProfile,
		ReasoningEffort: m.reasoningEffort,
		MaxToolCalls:    m.maxToolCalls,
		RequestTimeout:  m.runtimeConfig.ModelRequestTimeout,
		TurnTimeout:     m.runtimeConfig.TurnTimeout,
		RoundTimeout:    m.runtimeConfig.RoundTimeout,
		RemoteModel:     remote,
	})
	if err == nil && conversation != nil {
		m.runner = conversation
		if m.bottom != nil {
			m.bottom.setHasRunner(true)
		}
	}
}

func (m *bubbleModel) setPermissionMode(mode permission.Mode) error {
	if err := m.service.SetMode(mode); err != nil {
		return err
	}
	if m.coordinator != nil {
		m.coordinator.SetPermissionMode(mode)
	}
	return nil
}
