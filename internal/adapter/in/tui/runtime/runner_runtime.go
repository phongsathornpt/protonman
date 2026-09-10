package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/conversation"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type toolResultMsg struct {
	call   tool.Call
	result tool.Result
	err    error
}

func (m *bubbleModel) retainConversationMessages() {
	if m == nil {
		return
	}
	m.messages = conversation.Retain(m.messages, m.conversationRetention)
}

func (m *bubbleModel) startTool(call tool.Call) tea.Cmd {
	m.showWelcome = false
	slog.DebugContext(m.ctx, "tui direct tool started", "call_id", call.ID, "tool_name", call.Name, "argument_bytes", len(call.Arguments))
	m.messages = append(m.messages, model.Message{ID: model.NewMessageID(), Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: call.ID, Name: call.Name, Arguments: append([]byte(nil), call.Arguments...)}}})
	m.retainConversationMessages()
	m.busy = true
	m.busyStarted = time.Now()
	m.activity = "running " + call.Name
	m.appendToolCall(call)
	m.requestRelayout()
	ctx, cancel := context.WithCancel(m.ctx)
	m.turnCancel = cancel
	return func() tea.Msg {
		defer cancel()
		startedAt := time.Now()
		result, callErr := m.service.Call(ctx, call)
		attrs := []any{"call_id", call.ID, "tool_name", call.Name, "duration_ms", time.Since(startedAt).Milliseconds(), "success", callErr == nil, "error_type", errorType(callErr)}
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
	content, err := json.Marshal(result.ModelPayload())
	if err != nil {
		content = []byte(fmt.Sprintf(`{"call_id":%q,"tool_name":%q,"error":{"code":"execution_error","message":%q}}`, call.ID, call.Name, err.Error()))
	}
	m.messages = append(m.messages, model.Message{ID: model.NewMessageID(), Role: model.RoleTool, Content: string(content), ToolCallID: result.CallID, ToolName: result.ToolName})
	m.retainConversationMessages()
}

func (m *bubbleModel) reconfigureRunner() {
	if m.activeModel == "" || m.service == nil {
		m.runner = nil
		m.syncPromptPlaceholder()
		return
	}
	provName := m.activeProvider
	if provName == "" {
		provName = model.DefaultOpenCodeName
	}
	prov, ok := m.providers[strings.ToLower(provName)]
	hasValidAuth := ok && model.ProviderHasUsableAuth(provName, prov.BaseURL, prov.APIKey)
	if !hasValidAuth {
		loaded, err := (app.Providers{}).LoadConfigured(m.ctx, m.workDir)
		if err == nil {
			if m.providers == nil {
				m.providers = make(map[string]config.ProviderConfig)
			}
			for key, value := range loaded {
				m.providers[key] = value
			}
			prov, ok = m.providers[strings.ToLower(provName)]
			hasValidAuth = ok && model.ProviderHasUsableAuth(provName, prov.BaseURL, prov.APIKey)
		}
	}
	if !hasValidAuth {
		m.runner = nil
		m.syncPromptPlaceholder()
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
	conversation, err := app.BuildConversation(m.service, m.skills, m.agents, app.ConversationSpec{ProviderName: provName, ProviderType: prov.Type, BaseURL: prov.BaseURL, APIKey: prov.APIKey, ModelID: m.activeModel, SessionID: sessID, Workspace: m.workDir, ActiveGoal: m.activeGoal, AgentProfile: m.agentProfile, ReasoningEffort: m.reasoningEffort, MaxToolCalls: m.maxToolCalls, RequestTimeout: m.runtimeConfig.ModelRequestTimeout, TurnTimeout: m.runtimeConfig.TurnTimeout, RoundTimeout: m.runtimeConfig.RoundTimeout, RemoteModel: remote})
	if err != nil {
		m.appendError("failed to configure model runner: " + err.Error())
		m.runner = nil
		m.syncPromptPlaceholder()
		return
	}
	if conversation != nil {
		m.runner = conversation
		m.syncPromptPlaceholder()
	}
}

func (m *bubbleModel) setPermissionMode(mode permission.Mode) error {
	if err := m.service.SetMode(mode); err != nil {
		return err
	}
	m.agents.SetPermissionMode(mode)
	m.syncPromptPlaceholder()
	return nil
}

func (m *bubbleModel) syncPromptPlaceholder() {
	if m == nil || m.panes.bottom == nil {
		return
	}
	mode := permission.ModeAsk
	if m.service != nil {
		mode = m.service.Mode()
	}
	hasRunner := m.runner != nil
	m.panes.bottom.setPlaceholder(promptPlaceholder(hasRunner, mode, m.planMode))
}
