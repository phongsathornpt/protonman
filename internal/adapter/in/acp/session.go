package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/phongsathornpt/proton/internal/base/runtimepolicy"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/phongsathornpt/proton/internal/adapter/out/model"
	"github.com/phongsathornpt/proton/internal/app"
	"github.com/phongsathornpt/proton/internal/app/appdirs"
	"github.com/phongsathornpt/proton/internal/base/contextutil"
	"github.com/phongsathornpt/proton/internal/core/permission"
	"github.com/phongsathornpt/proton/internal/core/session"
	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/engine/toolcall"
	sdk "github.com/phongsathornpt/proton/proton-sdk"
)

const sessionPersistenceTimeout = runtimepolicy.SessionPersistenceTimeout

// Session represents an active ACP conversation thread.
type Session struct {
	id              string
	cwd             string
	workspaceKey    string
	workspaceName   string
	stateRevision   uint64
	service         *toolcall.Service
	registry        tool.Registry
	runner          app.Conversation
	sessionService  *app.Sessions
	reasoningEffort sdk.ReasoningEffort
	mcpServers      []MCPServerConfig
	resource        io.Closer

	mu        sync.Mutex
	messages  []model.Message
	active    bool
	cancelled bool
	cancel    context.CancelFunc
	nextID    uint64
}

// NewSession creates an ACP session with its own conversation history and tools.
func NewSession(
	id string,
	cwd string,
	service *toolcall.Service,
	registry tool.Registry,
	runner app.Conversation,
	sessionService *app.Sessions,
) *Session {
	reasoningEffort := sdk.ReasoningDefault
	if effort, explicit := app.ReasoningPolicy(runner); explicit {
		reasoningEffort = effort
	}
	workspaceName := filepath.Base(filepath.Clean(cwd))
	if cwd == "" {
		workspaceName = ""
	}
	return &Session{
		id: id, cwd: cwd, workspaceKey: session.WorkspaceKey(cwd), workspaceName: workspaceName,
		service: service, registry: registry, runner: runner, sessionService: sessionService,
		reasoningEffort: reasoningEffort, messages: make([]model.Message, 0),
	}
}

func (s *Session) matchMCPServers(configs []MCPServerConfig) error {
	if len(configs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !sameMCPServerConfigs(s.mcpServers, configs) {
		return fmt.Errorf("session %q MCP server configuration differs from the active session", s.id)
	}
	return nil
}

// Close releases session-owned transports and other external resources.
func (s *Session) Close() error {
	s.mu.Lock()
	resource := s.resource
	s.resource = nil
	s.mu.Unlock()
	if resource == nil {
		return nil
	}
	return resource.Close()
}

// ReasoningEffort returns the explicit session-local reasoning override.
func (s *Session) ReasoningEffort() sdk.ReasoningEffort {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reasoningEffort
}

// SetReasoningEffort replaces the session-local Proton loop with an independently configured clone.
func (s *Session) SetReasoningEffort(effort sdk.ReasoningEffort) error {
	if !effort.Valid() {
		return fmt.Errorf("invalid reasoning effort %q", effort)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clone, err := app.CloneConversationWithReasoning(s.runner, effort, effort != sdk.ReasoningDefault)
	if err != nil {
		return err
	}
	s.runner = clone
	s.reasoningEffort = effort
	return nil
}

// ID returns the unique session ID.
func (s *Session) ID() string {
	return s.id
}

// Cwd returns the working directory for this session.
func (s *Session) Cwd() string {
	return s.cwd
}

// Messages returns a copy of the current message history.
func (s *Session) Messages() []model.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return model.CloneMessages(s.messages)
}

// SetMessages replaces the current message history.
func (s *Session) SetMessages(messages []model.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = model.CloneMessages(messages)
}

// ExecutePrompt runs a prompt turn, streaming events in real time to notifier.
func (s *Session) ExecutePrompt(
	ctx context.Context,
	promptBlocks []ContentBlock,
	notifier func(RPCNotification) error,
) (SessionPromptResult, error) {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return SessionPromptResult{}, fmt.Errorf("session %q already has an active prompt", s.id)
	}
	promptCtx, cancel := context.WithCancel(ctx)
	s.active = true
	s.cancelled = false
	s.cancel = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.active = false
		s.cancel = nil
		s.mu.Unlock()
		cancel()
	}()

	userMsg := ContentBlocksToModelMessage(promptBlocks)
	trimmedPrompt := strings.TrimSpace(userMsg.TextContent())

	// Handle slash commands
	if strings.HasPrefix(trimmedPrompt, "/") {
		handled, result, err := s.handleSlashCommand(promptCtx, userMsg, trimmedPrompt, notifier)
		if handled {
			return result, err
		}
	}

	if s.runner == nil {
		return SessionPromptResult{}, fmt.Errorf("no model runner configured; please configure an LLM provider in %s", appdirs.UserConfigDisplay())
	}

	s.mu.Lock()
	baseMessageCount := len(s.messages)
	s.messages = append(s.messages, userMsg)
	history := model.CloneMessages(s.messages)
	s.mu.Unlock()

	var assistantText strings.Builder
	var assistantCalls []tool.Call
	var turnMessages []model.Message
	pendingToolCalls := make(map[string]tool.Call)

	sink := func(sinkCtx context.Context, event app.Event) error {
		if event.Kind != app.EventToolResult && event.Kind != app.EventFailed {
			select {
			case <-sinkCtx.Done():
				return sinkCtx.Err()
			case <-promptCtx.Done():
				return promptCtx.Err()
			default:
			}
		}

		switch event.Kind {
		case app.EventTextDelta:
			assistantText.WriteString(event.Text)
			return notifier(RPCNotification{
				JSONRPC: "2.0",
				Method:  "session/update",
				Params: map[string]any{
					"sessionId": s.id,
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content": map[string]any{
							"type": string(BlockTypeText),
							"text": event.Text,
						},
					},
				},
			})

		case app.EventToolCall:
			assistantCalls = append(assistantCalls, event.Call)
			pendingToolCalls[event.Call.ID] = event.Call
			locations := LocationsForToolCall(event.Call)
			payload := map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    event.Call.ID,
				"title":         TitleForToolCall(event.Call),
				"kind":          string(ToolKindForName(event.Call.Name)),
				"status":        string(ToolCallStatusInProgress),
			}
			if len(locations) > 0 {
				payload["locations"] = locations
			}
			return notifier(RPCNotification{
				JSONRPC: "2.0",
				Method:  "session/update",
				Params: map[string]any{
					"sessionId": s.id,
					"update":    payload,
				},
			})

		case app.EventToolResult:
			callID := event.Result.CallID
			if callID == "" {
				callID = event.Call.ID
			}
			delete(pendingToolCalls, callID)
			return notifyToolCallUpdate(notifier, s.id, callID, event.Result, event.Err)

		case app.EventFailed:
			ids := make([]string, 0, len(pendingToolCalls))
			for callID := range pendingToolCalls {
				ids = append(ids, callID)
			}
			sort.Strings(ids)
			for _, callID := range ids {
				call := pendingToolCalls[callID]
				result := tool.Result{
					CallID:   callID,
					ToolName: call.Name,
					Failure:  tool.FailureFromError(event.Err),
				}
				if err := notifyToolCallUpdate(notifier, s.id, callID, result, event.Err); err != nil {
					return err
				}
				delete(pendingToolCalls, callID)
			}

		case app.EventCompleted:
			if event.Message.ToolCalls != nil || event.Text != "" {
				turnMessages = append(turnMessages, event.Message)
			}
		}
		return nil
	}

	result, err := s.runner.Run(promptCtx, history, sink)

	s.mu.Lock()
	wasCancelled := s.cancelled || promptCtx.Err() != nil
	if err != nil || wasCancelled {
		if baseMessageCount <= len(s.messages) {
			s.messages = s.messages[:baseMessageCount]
		}
	} else if len(result.Messages) > 0 {
		s.messages = append(s.messages, result.Messages...)
	} else if len(turnMessages) > 0 {
		s.messages = append(s.messages, turnMessages...)
	} else if assistantText.Len() > 0 || len(assistantCalls) > 0 {
		s.messages = append(s.messages, model.Message{
			Role:      model.RoleAssistant,
			Content:   assistantText.String(),
			ToolCalls: toModelToolCalls(assistantCalls),
		})
	}
	s.mu.Unlock()

	saveErr := s.saveStateDetached(promptCtx)
	if wasCancelled {
		if saveErr != nil {
			return SessionPromptResult{}, fmt.Errorf("save canceled session %q: %w", s.id, saveErr)
		}
		return SessionPromptResult{StopReason: StopReasonCancelled}, nil
	}
	if err != nil {
		if saveErr != nil {
			return SessionPromptResult{}, errors.Join(err, fmt.Errorf("save session %q: %w", s.id, saveErr))
		}
		return SessionPromptResult{}, err
	}
	if saveErr != nil {
		return SessionPromptResult{}, fmt.Errorf("save session %q: %w", s.id, saveErr)
	}

	return SessionPromptResult{StopReason: StopReasonEndTurn}, nil
}

// Cancel interrupts an active prompt turn.
func (s *Session) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active && s.cancel != nil {
		s.cancelled = true
		s.cancel()
	}
}

// ReplayHistory streams previous conversation messages via session/update.
func (s *Session) ReplayHistory(notifier func(RPCNotification) error) error {
	s.mu.Lock()
	messages := model.CloneMessages(s.messages)
	s.mu.Unlock()

	for _, msg := range messages {
		var updateType string
		switch msg.Role {
		case model.RoleUser:
			updateType = "user_message_chunk"
		case model.RoleAssistant:
			updateType = "agent_message_chunk"
		default:
			continue
		}

		text := msg.TextContent()
		if text == "" {
			continue
		}

		err := notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": updateType,
					"content": map[string]any{
						"type": string(BlockTypeText),
						"text": text,
					},
				},
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) handleSlashCommand(
	ctx context.Context,
	userMsg model.Message,
	cmd string,
	notifier func(RPCNotification) error,
) (bool, SessionPromptResult, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false, SessionPromptResult{}, nil
	}

	switch parts[0] {
	case "/new":
		s.mu.Lock()
		s.messages = nil
		s.mu.Unlock()
		if err := s.saveStateDetached(ctx); err != nil {
			return true, SessionPromptResult{}, fmt.Errorf("save session %q: %w", s.id, err)
		}
		_ = notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content": map[string]any{
						"type": string(BlockTypeText),
						"text": "Started fresh conversation thread.",
					},
				},
			},
		})
		return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil

	case "/mode":
		if len(parts) > 1 {
			mode, err := permission.ParseMode(parts[1])
			if err == nil {
				s.service.SetMode(mode)
				_ = notifier(RPCNotification{
					JSONRPC: "2.0",
					Method:  "session/update",
					Params: map[string]any{
						"sessionId": s.id,
						"update": map[string]any{
							"sessionUpdate": "current_mode_update",
							"modeId":        mode.String(),
						},
					},
				})
				_ = notifier(RPCNotification{
					JSONRPC: "2.0",
					Method:  "session/update",
					Params: map[string]any{
						"sessionId": s.id,
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content": map[string]any{
								"type": string(BlockTypeText),
								"text": fmt.Sprintf("Switched permission mode to **%s**.", mode.String()),
							},
						},
					},
				})
				return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil
			}
		}
	case "/ask":
		s.service.SetMode(permission.ModeAsk)
		_ = notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": "current_mode_update",
					"modeId":        "ask",
				},
			},
		})
		_ = notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content": map[string]any{
						"type": string(BlockTypeText),
						"text": "Switched to **ask** mode.",
					},
				},
			},
		})
		return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil

	case "/plan":
		s.service.SetMode(permission.ModeDeny)
		_ = notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": "current_mode_update",
					"modeId":        "plan",
				},
			},
		})
		_ = notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content": map[string]any{
						"type": string(BlockTypeText),
						"text": "Switched to read-only **plan** mode.",
					},
				},
			},
		})
		return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil

	case "/always-approve":
		s.service.SetMode(permission.ModeAlwaysApprove)
		_ = notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": "current_mode_update",
					"modeId":        "always-approve",
				},
			},
		})
		_ = notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content": map[string]any{
						"type": string(BlockTypeText),
						"text": "Switched to **always-approve** mode.",
					},
				},
			},
		})
		return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil

	case "/reasoning", "/thinking":
		if len(parts) == 1 {
			label := "auto"
			if effort := s.ReasoningEffort(); effort != sdk.ReasoningDefault {
				label = string(effort)
			}
			_ = notifier(agentMessageNotification(s.id, "Reasoning override: **"+label+"**."))
			return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil
		}
		effort, err := sdk.ParseReasoningEffort(parts[1])
		if err != nil {
			return true, SessionPromptResult{}, fmt.Errorf("invalid reasoning effort: use auto, none, low, medium, high, xhigh, or max")
		}
		if err := s.SetReasoningEffort(effort); err != nil {
			return true, SessionPromptResult{}, err
		}
		if err := s.saveStateDetached(ctx); err != nil {
			return true, SessionPromptResult{}, fmt.Errorf("save session %q: %w", s.id, err)
		}
		label := "auto"
		if effort != sdk.ReasoningDefault {
			label = string(effort)
		}
		_ = notifier(agentMessageNotification(s.id, "Reasoning override set to **"+label+"**."))
		return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil

	case "/tools":
		var b strings.Builder
		b.WriteString("### Registered Tools\n\n")
		for _, def := range s.registry.Definitions() {
			b.WriteString(fmt.Sprintf("- **`%s`**: %s\n", def.Name, def.Description))
		}
		_ = notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content": map[string]any{
						"type": string(BlockTypeText),
						"text": b.String(),
					},
				},
			},
		})
		return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil

	case "/call":
		body := strings.TrimSpace(strings.TrimPrefix(cmd, "/"))
		cmdParts := strings.SplitN(body, " ", 3)
		if len(cmdParts) < 2 || strings.TrimSpace(cmdParts[1]) == "" {
			return true, SessionPromptResult{}, fmt.Errorf("usage: /call <tool> <json>")
		}
		toolName := strings.TrimSpace(cmdParts[1])
		arguments := "{}"
		if len(cmdParts) == 3 && strings.TrimSpace(cmdParts[2]) != "" {
			arguments = strings.TrimSpace(cmdParts[2])
		}

		s.mu.Lock()
		s.nextID++
		callID := fmt.Sprintf("acp-%d", s.nextID)
		s.mu.Unlock()

		call, err := tool.NewCall(callID, toolName, json.RawMessage(arguments))
		if err != nil {
			return true, SessionPromptResult{}, err
		}

		locations := LocationsForToolCall(call)
		payload := map[string]any{
			"sessionUpdate": "tool_call",
			"toolCallId":    call.ID,
			"title":         TitleForToolCall(call),
			"kind":          string(ToolKindForName(call.Name)),
			"status":        string(ToolCallStatusInProgress),
			"rawInput":      call.Arguments,
		}
		if len(locations) > 0 {
			payload["locations"] = locations
		}
		if err := notifier(RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": s.id,
				"update":    payload,
			},
		}); err != nil {
			return true, SessionPromptResult{}, fmt.Errorf("notify tool call %q: %w", call.ID, err)
		}

		result, callErr := s.service.Call(ctx, call)
		if err := notifyToolCallUpdate(notifier, s.id, call.ID, result, callErr); err != nil {
			return true, SessionPromptResult{}, err
		}

		s.mu.Lock()
		wasCancelled := s.cancelled || ctx.Err() != nil
		s.mu.Unlock()
		if wasCancelled {
			return true, SessionPromptResult{StopReason: StopReasonCancelled}, nil
		}

		output := result.Output
		if output == "" && callErr != nil {
			output = callErr.Error()
		}
		if output != "" {
			_ = notifier(RPCNotification{
				JSONRPC: "2.0",
				Method:  "session/update",
				Params: map[string]any{
					"sessionId": s.id,
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content": map[string]any{
							"type": string(BlockTypeText),
							"text": output,
						},
					},
				},
			})
		}

		s.mu.Lock()
		s.messages = append(s.messages,
			userMsg,
			model.Message{
				Role: model.RoleAssistant,
				ToolCalls: []model.ToolCall{
					{
						ID:        call.ID,
						Name:      call.Name,
						Arguments: call.Arguments,
					},
				},
			},
			model.Message{
				Role:       model.RoleTool,
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    result.Output,
			},
		)
		s.mu.Unlock()
		if err := s.saveStateDetached(ctx); err != nil {
			return true, SessionPromptResult{}, fmt.Errorf("save session %q: %w", s.id, err)
		}

		if callErr != nil {
			return true, SessionPromptResult{}, callErr
		}
		return true, SessionPromptResult{StopReason: StopReasonEndTurn}, nil
	}

	return false, SessionPromptResult{}, nil
}

func agentMessageNotification(sessionID, text string) RPCNotification {
	return RPCNotification{
		JSONRPC: "2.0", Method: "session/update",
		Params: map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": string(BlockTypeText), "text": text},
			},
		},
	}
}

func notifyToolCallUpdate(
	notifier func(RPCNotification) error,
	sessionID string,
	callID string,
	result tool.Result,
	callErr error,
) error {
	status := ToolCallStatusCompleted
	if result.Failure != nil || result.Denied || callErr != nil {
		status = ToolCallStatusFailed
	}
	text := result.Output
	if text == "" && result.Failure != nil {
		text = result.Failure.Message
	}
	if text == "" && callErr != nil {
		text = callErr.Error()
	}
	return notifier(RPCNotification{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params: map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    callID,
				"status":        string(status),
				"content": []map[string]any{
					{
						"type": "content",
						"content": map[string]any{
							"type": string(BlockTypeText),
							"text": text,
						},
					},
				},
			},
		},
	})
}

func (s *Session) saveStateDetached(parent context.Context) error {
	ctx, cancel := contextutil.DetachedTimeout(parent, sessionPersistenceTimeout)
	defer cancel()
	return s.saveState(ctx)
}

func (s *Session) saveState(ctx context.Context) error {
	if s.sessionService == nil {
		return nil
	}
	s.mu.Lock()
	messages := model.CloneMessages(s.messages)
	reasoningEffort := s.reasoningEffort
	stateRevision := s.stateRevision
	s.mu.Unlock()
	reasoningSetting := "auto"
	if reasoningEffort != sdk.ReasoningDefault {
		reasoningSetting = string(reasoningEffort)
	}

	err := s.sessionService.Save(ctx, s.id, session.State{
		SessionID:       s.id,
		Revision:        stateRevision,
		WorkspaceKey:    s.workspaceKey,
		WorkspaceName:   s.workspaceName,
		PermissionMode:  s.service.Mode().String(),
		ReasoningEffort: reasoningSetting,
		Messages:        session.FromModelMessages(messages),
		UpdatedAt:       time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.stateRevision == stateRevision {
		s.stateRevision = stateRevision + 1
	}
	s.mu.Unlock()
	return nil
}

func toModelToolCalls(calls []tool.Call) []model.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]model.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, model.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

// WriteJSON sends a line-delimited JSON payload safely to writer.
func WriteJSON(output io.Writer, mu *sync.Mutex, value any) error {
	payload, err := jsonMarshal(value)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	mu.Lock()
	defer mu.Unlock()
	_, err = output.Write(payload)
	return err
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
