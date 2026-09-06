package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/agentprompt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/appdirs"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

func (m *bubbleModel) submit() tea.Cmd {
	m.syncLegacyToComponents()
	prompt := m.bottom.prompt()
	line := strings.TrimSpace(prompt.Value())
	if m.bottom.bashMode() {
		prompt.Reset()
		m.bottom.remove(slashViewID)
		if line == "" {
			m.setBashMode(false)
			return nil
		}
		if m.busy || m.hasPermissionView() {
			m.queue = append(m.queue, "!"+line)
			m.appendMuted(fmt.Sprintf("queued (%d): !%s", len(m.queue), line))
			m.refreshViewport()
			return nil
		}
		m.setBashMode(false)
		return m.dispatchBang(line)
	}
	if line == "" {
		return nil
	}
	prompt.Reset()
	m.bottom.remove(slashViewID)
	if m.busy || m.hasPermissionView() {
		m.queue = append(m.queue, line)
		m.appendMuted(fmt.Sprintf("queued (%d): %s", len(m.queue), line))
		m.refreshViewport()
		return nil
	}
	return m.dispatch(line)
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
		// Reload from disk in case config was written or updated
		dirs, resolveErr := appdirs.Resolve("")
		if resolveErr != nil {
			return
		}
		homeDir := dirs.Home
		loaded, err := config.Load(m.ctx, config.Options{
			HomeDir: homeDir,
			WorkDir: m.workDir,
		})
		if err == nil {
			if m.providers == nil {
				m.providers = make(map[string]config.ProviderConfig)
			}
			for k, v := range loaded.Providers {
				m.providers[k] = v
			}
			prov, ok = m.providers[strings.ToLower(provName)]
			hasValidAuth = ok && model.ProviderHasUsableAuth(provName, prov.BaseURL, prov.APIKey)
		}
	}
	if !hasValidAuth {
		return
	}
	baseURL := model.ResolveProviderBaseURLForProtocol(provName, prov.Type, prov.BaseURL)
	sessID := m.sessionID
	if sessID == "" && m.workDir != "" {
		sessID = "workspace-" + m.workDir
	}
	var clientOpts []model.ClientOption
	clientOpts = append(clientOpts, model.WithRequestTimeout(m.runtimeConfig.ModelRequestTimeout))
	if remoteModel, ok := m.activeRemoteModel(); ok {
		clientOpts = append(clientOpts, model.WithRemoteModelProfile(provName, remoteModel))
	}
	if sessID != "" {
		clientOpts = append(clientOpts, model.WithSessionID(sessID))
	}
	languageModel := model.NewProviderLanguageModel(provName, prov.Type, baseURL, prov.APIKey, m.activeModel, clientOpts...)
	promptSpec := agentprompt.Spec{Workspace: m.workDir}
	if profileName := strings.TrimSpace(m.agentProfile); profileName != "" {
		if profile, err := agent.ParseProfile(profileName); err == nil {
			promptSpec.Profile = string(profile)
			promptSpec.Role = agent.RolePromptForProfile(profile)
		}
	}
	var opts []applicationturn.Option
	opts = append(opts, applicationturn.WithSystemPromptSpec(promptSpec))
	if m.skills != nil {
		opts = append(opts, applicationturn.WithSkillRegistry(m.skills))
	}
	opts = append(opts, applicationturn.WithMaxRounds(m.maxRounds))
	opts = append(opts, applicationturn.WithMaxToolCalls(m.maxToolCalls))
	opts = append(opts, applicationturn.WithTurnTimeout(m.runtimeConfig.TurnTimeout))
	opts = append(opts, applicationturn.WithRoundTimeout(m.runtimeConfig.RoundTimeout))
	if strings.TrimSpace(m.agentProfile) != "" {
		opts = append(opts, applicationturn.WithRequireInitialToolUse(true))
	}
	loop, err := applicationturn.NewLoop(languageModel, m.service, opts...)
	if err == nil {
		m.runner = loop
		if m.coordinator != nil {
			m.coordinator.SetLanguageModel(languageModel)
		}
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
