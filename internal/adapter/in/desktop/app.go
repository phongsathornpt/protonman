// Package desktop contains the framework-independent controller used by the
// Wails desktop application. The controller owns the ACP client lifecycle and
// projects ACP notifications into desktop feature state; React only consumes
// its typed snapshot and commands.
package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	featuredesktop "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	defaultWorkspace = "."
	EventSnapshot    = "desktop:snapshot"
)

var ErrNotConnected = errors.New("desktop ACP client is not connected")

type Snapshot struct {
	Status          string            `json:"status"`
	Connection      string            `json:"connection"`
	ActiveSessionID string            `json:"activeSessionId"`
	Sessions        []SessionSnapshot `json:"sessions"`
	PermissionInbox []PermissionView  `json:"permissionInbox"`
}

type SessionSnapshot struct {
	ID                string            `json:"id"`
	Title             string            `json:"title"`
	Workspace         string            `json:"workspace"`
	Mode              string            `json:"mode"`
	Status            string            `json:"status"`
	Timeline          []TimelineView    `json:"timeline"`
	Subagents         []SubagentView    `json:"subagents"`
	Context           ContextView       `json:"context"`
	Runtime           RuntimeView       `json:"runtime"`
	ProviderOptions   []ModelOptionView `json:"providerOptions"`
	ModelOptions      []ModelOptionView `json:"modelOptions"`
	ModelOptionsError string            `json:"modelOptionsError,omitempty"`
}

type TimelineView struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

type PermissionView struct {
	RequestID string                 `json:"requestId"`
	SessionID string                 `json:"sessionId"`
	Title     string                 `json:"title"`
	Detail    string                 `json:"detail"`
	Options   []PermissionViewOption `json:"options"`
}

type PermissionViewOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type SubagentView struct {
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Task    string `json:"task"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

type RuntimeView struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Reasoning      string `json:"reasoning"`
	LowConcurrency string `json:"lowConcurrency"`
}

type ModelOptionView struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type MCPServerView struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

type ContextView struct {
	Goal   string     `json:"goal"`
	Todo   TodoView   `json:"todo"`
	Memory MemoryView `json:"memory"`
}

type TodoView struct {
	Revision uint64         `json:"revision"`
	Items    []TodoItemView `json:"items"`
}

type TodoItemView struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

type TodoOperationView struct {
	Op     string `json:"op"`
	ID     string `json:"id"`
	Text   string `json:"text,omitempty"`
	Status string `json:"status,omitempty"`
}

type MemoryView struct {
	WorkspaceKey string            `json:"workspaceKey"`
	Workspace    []MemoryEntryView `json:"workspace"`
	Global       []MemoryEntryView `json:"global"`
}

type MemoryEntryView struct {
	ID         string  `json:"id"`
	Scope      string  `json:"scope"`
	Kind       string  `json:"kind"`
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	UsageCount uint64  `json:"usageCount"`
}

type Controller struct {
	mu                sync.RWMutex
	sessionLoadMu     sync.Mutex
	state             featuredesktop.State
	status            string
	connection        string
	client            *acpclient.Client
	ctx               context.Context
	cancel            context.CancelFunc
	workspace         string
	loadingSessionID  string
	pending           map[string]chan permissionChoice
	requestSeq        atomic.Uint64
	subs              map[chan Snapshot]struct{}
	closeOnce         sync.Once
	closed            chan struct{}
	providerOptions   map[string][]ModelOptionView
	modelOptions      map[string][]ModelOptionView
	modelOptionsError map[string]string
}

type permissionChoice struct{ optionID string }

type initializeParams struct {
	ProtocolVersion    int `json:"protocolVersion"`
	ClientCapabilities struct {
		Terminal bool `json:"terminal"`
	} `json:"clientCapabilities"`
	ClientInfo struct {
		Name    string `json:"name"`
		Title   string `json:"title"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type sessionNewResult struct {
	SessionID     string                `json:"sessionId"`
	ConfigOptions []sessionConfigOption `json:"configOptions"`
}

type sessionLoadResult struct {
	ConfigOptions []sessionConfigOption `json:"configOptions"`
}

type sessionConfigResult struct {
	ConfigOptions []sessionConfigOption `json:"configOptions"`
}

type sessionConfigOption struct {
	ID      string            `json:"id"`
	Options []ModelOptionView `json:"options"`
	Error   string            `json:"error"`
}

type sessionListResult struct {
	Sessions []struct {
		SessionID     string `json:"sessionId"`
		Cwd           string `json:"cwd"`
		Title         string `json:"title"`
		WorkspaceKey  string `json:"workspaceKey"`
		WorkspaceName string `json:"workspaceName"`
	} `json:"sessions"`
}

type sessionPromptParams struct {
	SessionID string `json:"sessionId"`
	Prompt    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"prompt"`
}

type sessionUpdate struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		Kind       string          `json:"sessionUpdate"`
		ToolCallID string          `json:"toolCallId"`
		Title      string          `json:"title"`
		Status     string          `json:"status"`
		ModeID     string          `json:"modeId"`
		AgentID    string          `json:"agentId"`
		Profile    string          `json:"profile"`
		Task       string          `json:"task"`
		Summary    string          `json:"summary"`
		Content    json.RawMessage `json:"content"`
	} `json:"update"`
}

type runtimeResult struct {
	SessionID      string `json:"sessionId"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Reasoning      string `json:"reasoning"`
	LowConcurrency string `json:"lowConcurrency"`
}

type contextResult struct {
	SessionID string `json:"sessionId"`
	Goal      string `json:"goal"`
	Todo      struct {
		Revision uint64 `json:"revision"`
		Items    []struct {
			ID     string `json:"id"`
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"items"`
	} `json:"todo"`
}

type todoUpdateResult struct {
	SessionID string `json:"sessionId"`
	Todo      struct {
		Revision uint64 `json:"revision"`
		Items    []struct {
			ID     string `json:"id"`
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"items"`
	} `json:"todo"`
}

type memoryResult struct {
	SessionID    string              `json:"sessionId"`
	WorkspaceKey string              `json:"workspaceKey"`
	Workspace    []memoryEntryResult `json:"workspace"`
	Global       []memoryEntryResult `json:"global"`
}

type memoryForgetResult struct {
	SessionID string `json:"sessionId"`
	Scope     string `json:"scope"`
	Removed   int    `json:"removed"`
}

type memoryEntryResult struct {
	ID         string  `json:"id"`
	Scope      string  `json:"scope"`
	Kind       string  `json:"kind"`
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	UsageCount uint64  `json:"usageCount"`
}

func (c *Controller) SetMode(ctx context.Context, sessionID, modeID string) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	sessionID = strings.TrimSpace(sessionID)
	modeID = strings.TrimSpace(modeID)
	if sessionID == "" || modeID == "" {
		return c.Snapshot(ctx), errors.New("session and mode are required")
	}
	var result struct{}
	if err := client.Call(ctx, "session/set_mode", map[string]string{"sessionId": sessionID, "modeId": modeID}, &result); err != nil {
		return c.Snapshot(ctx), err
	}
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionModeUpdated, SessionID: sessionID, Mode: modeID})
	return c.Snapshot(ctx), nil
}

type permissionParams struct {
	SessionID string         `json:"sessionId"`
	ToolCall  map[string]any `json:"toolCall"`
	Options   []struct {
		ID   string `json:"optionId"`
		Name string `json:"name"`
		Kind string `json:"kind"`
	} `json:"options"`
}

func NewController() *Controller {
	return &Controller{
		status:            "Ready to connect",
		connection:        "disconnected",
		state:             featuredesktop.State{AgentHealth: map[string]featuredesktop.AgentHealthState{}},
		pending:           make(map[string]chan permissionChoice),
		subs:              make(map[chan Snapshot]struct{}),
		closed:            make(chan struct{}),
		providerOptions:   make(map[string][]ModelOptionView),
		modelOptions:      make(map[string][]ModelOptionView),
		modelOptionsError: make(map[string]string),
	}
}

func (c *Controller) Bootstrap(binary, workspace string) (Snapshot, error) {
	workspace = normalizeWorkspace(workspace)
	if binary == "" {
		binary = resolveBinary()
	}
	if binary == "" {
		return c.updateStatus("Set PROTONMAN_BINARY or build the CLI binary first", "error"), errors.New("protonman ACP binary was not found")
	}

	c.mu.Lock()
	if c.client != nil {
		snapshot := c.snapshotLocked()
		c.mu.Unlock()
		return snapshot, nil
	}
	c.status = "Connecting to Protonman"
	c.connection = "connecting"
	c.workspace = workspace
	ctx, cancel := context.WithCancel(context.Background())
	c.ctx, c.cancel = ctx, cancel
	c.mu.Unlock()
	c.publish()

	client, err := acpclient.Start(ctx, binary, c.handleEvent)
	if err != nil {
		cancel()
		return c.updateStatus("Unable to start Protonman", "error"), err
	}
	client.SetRequestHandler(c.handleRequest)

	var initialized any
	params := initializeParams{ProtocolVersion: 1}
	params.ClientInfo = struct {
		Name    string `json:"name"`
		Title   string `json:"title"`
		Version string `json:"version"`
	}{Name: "protonman-desktop", Title: "Protonman Desktop", Version: "dev"}
	if err := client.Call(ctx, "initialize", params, &initialized); err != nil {
		_ = client.Close()
		return c.updateStatus("ACP initialization failed", "error"), err
	}

	c.mu.Lock()
	c.client = client
	c.status = "Connected"
	c.connection = "connected"
	c.mu.Unlock()
	c.publish()
	go c.watchClient(client)

	if err := c.refreshSessions(ctx, workspace); err != nil {
		c.abortConnection(client)
		return c.updateStatus("Connected, but sessions could not be loaded", "error"), err
	}
	c.mu.RLock()
	firstSessionID := ""
	if len(c.state.Sessions) > 0 {
		firstSessionID = c.state.Sessions[0].ID
	}
	c.mu.RUnlock()
	if firstSessionID != "" {
		if _, err := c.SelectSession(ctx, firstSessionID); err != nil {
			c.abortConnection(client)
			return c.updateStatus("Connected, but the session could not be loaded", "error"), err
		}
	}
	return c.Snapshot(context.Background()), nil
}

func (c *Controller) NewSession(ctx context.Context, workspace string, additionalDirectories []string, mcpServers []MCPServerView) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	workspace = normalizeWorkspace(workspace)
	var result sessionNewResult
	if err := client.Call(ctx, "session/new", map[string]any{
		"cwd":                   workspace,
		"additionalDirectories": additionalDirectories,
		"mcpServers":            mcpServers,
	}, &result); err != nil {
		return c.Snapshot(ctx), err
	}
	c.mu.Lock()
	c.storeConfigOptionsLocked(result.SessionID, result.ConfigOptions)
	c.state = featuredesktop.Reduce(c.state, featuredesktop.Event{Kind: featuredesktop.EventSessionsReplaced, Sessions: appendSession(c.state.Sessions, featuredesktop.SessionState{ID: result.SessionID, Workspace: workspace, WorkspaceName: filepath.Base(workspace), Status: featuredesktop.TaskIdle})})
	c.state = featuredesktop.Reduce(c.state, featuredesktop.Event{Kind: featuredesktop.EventSessionSelected, SessionID: result.SessionID})
	c.mu.Unlock()
	c.publish()
	c.refreshSessionMetadata(ctx, result.SessionID)
	return c.Snapshot(ctx), nil
}

func (c *Controller) SelectSession(ctx context.Context, sessionID string) (Snapshot, error) {
	c.sessionLoadMu.Lock()
	defer c.sessionLoadMu.Unlock()

	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return c.Snapshot(ctx), errors.New("session is required")
	}
	c.mu.RLock()
	workspace := c.workspace
	for _, session := range c.state.Sessions {
		if session.ID == sessionID && session.Workspace != "" {
			workspace = session.Workspace
			break
		}
	}
	c.mu.RUnlock()
	var result sessionLoadResult
	c.mu.Lock()
	c.loadingSessionID = sessionID
	c.mu.Unlock()
	if err := client.Call(ctx, "session/load", map[string]any{"sessionId": sessionID, "cwd": workspace}, &result); err != nil {
		c.mu.Lock()
		if c.loadingSessionID == sessionID {
			c.loadingSessionID = ""
		}
		c.mu.Unlock()
		return c.Snapshot(ctx), err
	}
	c.mu.Lock()
	c.storeConfigOptionsLocked(sessionID, result.ConfigOptions)
	if c.loadingSessionID == sessionID {
		c.state = featuredesktop.Reduce(c.state, featuredesktop.Event{Kind: featuredesktop.EventSessionTimelineReset, SessionID: sessionID})
		c.loadingSessionID = ""
	}
	c.mu.Unlock()
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionSelected, SessionID: sessionID})
	c.refreshSessionMetadata(ctx, sessionID)
	return c.Snapshot(ctx), nil
}

func (c *Controller) SetReasoning(ctx context.Context, sessionID, reasoning string) (Snapshot, error) {
	return c.setRuntimeValue(ctx, "protonman/session/set_reasoning", map[string]string{"sessionId": sessionID, "reasoning": reasoning})
}

func (c *Controller) SetModel(ctx context.Context, sessionID, modelID string) (Snapshot, error) {
	c.mu.RLock()
	provider := ""
	for _, session := range c.state.Sessions {
		if session.ID == sessionID {
			provider = session.Runtime.Provider
			break
		}
	}
	c.mu.RUnlock()
	if strings.TrimSpace(provider) == "" {
		return c.Snapshot(ctx), errors.New("active provider is not available")
	}
	return c.setRuntimeValue(ctx, "protonman/session/set_model", map[string]string{
		"sessionId": sessionID,
		"provider":  provider,
		"model":     modelID,
	})
}

func (c *Controller) SetProvider(ctx context.Context, sessionID, provider string) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	var result sessionConfigResult
	params := map[string]any{
		"sessionId": sessionID,
		"configId":  "provider",
		"type":      "select",
		"value":     provider,
	}
	if err := client.Call(ctx, "session/set_config_option", params, &result); err != nil {
		return c.Snapshot(ctx), err
	}
	c.storeConfigOptions(sessionID, result.ConfigOptions)
	return c.refreshSessionRuntime(ctx, sessionID)
}

func (c *Controller) SetLowConcurrency(ctx context.Context, sessionID, setting string) (Snapshot, error) {
	return c.setRuntimeValue(ctx, "protonman/session/set_low_concurrency", map[string]string{"sessionId": sessionID, "lowConcurrency": setting})
}

func (c *Controller) ForgetMemory(ctx context.Context, sessionID, scope, memoryID string) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	var result memoryForgetResult
	params := map[string]any{
		"sessionId": sessionID,
		"scope":     scope,
		"ids":       []string{memoryID},
	}
	if err := client.Call(ctx, "protonman/session/memory/forget", params, &result); err != nil {
		return c.Snapshot(ctx), err
	}
	c.refreshSessionMetadata(ctx, sessionID)
	return c.Snapshot(ctx), nil
}

func (c *Controller) UpdateTodoStatus(ctx context.Context, sessionID string, revision uint64, itemID, status string) (Snapshot, error) {
	return c.PatchTodo(ctx, sessionID, revision, TodoOperationView{Op: "set_status", ID: itemID, Status: status})
}

func (c *Controller) PatchTodo(ctx context.Context, sessionID string, revision uint64, operation TodoOperationView) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	var result todoUpdateResult
	params := map[string]any{
		"sessionId":  sessionID,
		"revision":   revision,
		"operations": []TodoOperationView{operation},
	}
	if err := client.Call(ctx, "protonman/session/todo/patch", params, &result); err != nil {
		return c.Snapshot(ctx), err
	}
	items := make([]featuredesktop.TodoItemState, 0, len(result.Todo.Items))
	for _, item := range result.Todo.Items {
		items = append(items, featuredesktop.TodoItemState{ID: item.ID, Text: item.Text, Status: item.Status})
	}
	goal := ""
	c.mu.RLock()
	for _, session := range c.state.Sessions {
		if session.ID == sessionID {
			goal = session.Context.Goal
			break
		}
	}
	c.mu.RUnlock()
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionContextUpdated, SessionID: sessionID, Context: featuredesktop.SessionContextState{Goal: goal, Todo: featuredesktop.TodoState{Revision: result.Todo.Revision, Items: items}}})
	return c.Snapshot(ctx), nil
}

func (c *Controller) CloseSession(ctx context.Context, sessionID string) (Snapshot, error) {
	return c.removeSession(ctx, sessionID, "session/close")
}

func (c *Controller) DeleteSession(ctx context.Context, sessionID string) (Snapshot, error) {
	return c.removeSession(ctx, sessionID, "session/delete")
}

func (c *Controller) removeSession(ctx context.Context, sessionID, method string) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return c.Snapshot(ctx), errors.New("session is required")
	}
	wasActive := c.isActiveSession(sessionID)
	if err := client.Call(ctx, method, map[string]string{"sessionId": sessionID}, &struct{}{}); err != nil {
		return c.Snapshot(ctx), err
	}
	if err := c.refreshSessions(ctx, c.workspace); err != nil {
		return c.Snapshot(ctx), err
	}
	if wasActive && method == "session/delete" {
		if next := c.firstSessionID(); next != "" {
			return c.SelectSession(ctx, next)
		}
	}
	if wasActive && method == "session/close" {
		c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionDeselected})
	}
	return c.Snapshot(ctx), nil
}

func (c *Controller) isActiveSession(sessionID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.ActiveSessionID == sessionID
}

func (c *Controller) firstSessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.state.Sessions) == 0 {
		return ""
	}
	return c.state.Sessions[0].ID
}

func (c *Controller) setRuntimeValue(ctx context.Context, method string, params map[string]string) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	var result runtimeResult
	if err := client.Call(ctx, method, params, &result); err != nil {
		return c.Snapshot(ctx), err
	}
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionRuntimeUpdated, SessionID: result.SessionID, Runtime: featuredesktop.RuntimeSettingsState{Provider: result.Provider, Model: result.Model, Reasoning: result.Reasoning, LowConcurrency: result.LowConcurrency}})
	return c.Snapshot(ctx), nil
}

func (c *Controller) refreshSessionRuntime(ctx context.Context, sessionID string) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	var result runtimeResult
	if err := client.Call(ctx, "protonman/session/runtime", map[string]string{"sessionId": sessionID}, &result); err != nil {
		return c.Snapshot(ctx), err
	}
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionRuntimeUpdated, SessionID: sessionID, Runtime: featuredesktop.RuntimeSettingsState{Provider: result.Provider, Model: result.Model, Reasoning: result.Reasoning, LowConcurrency: result.LowConcurrency}})
	return c.Snapshot(ctx), nil
}

func (c *Controller) SendPrompt(ctx context.Context, sessionID, text string) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	text = strings.TrimSpace(text)
	if sessionID == "" || text == "" {
		return c.Snapshot(ctx), errors.New("session and prompt are required")
	}
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventPromptStarted, SessionID: sessionID})
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventTimelineAppended, SessionID: sessionID, Item: featuredesktop.TimelineItem{Kind: featuredesktop.TimelineUser, ID: c.nextID("user"), Text: text}})

	params := sessionPromptParams{SessionID: sessionID}
	params.Prompt = append(params.Prompt, struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{Type: "text", Text: text})
	var result struct{}
	err = client.Call(ctx, "session/prompt", params, &result)
	if err != nil {
		c.reduce(featuredesktop.Event{Kind: featuredesktop.EventPromptFailed, SessionID: sessionID})
		return c.Snapshot(ctx), err
	}
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventPromptCompleted, SessionID: sessionID})
	return c.Snapshot(ctx), nil
}

func (c *Controller) CancelPrompt(ctx context.Context, sessionID string) (Snapshot, error) {
	client, err := c.connectedClient()
	if err != nil {
		return c.Snapshot(ctx), err
	}
	if err := client.Call(ctx, "session/cancel", map[string]string{"sessionId": sessionID}, &struct{}{}); err != nil {
		return c.Snapshot(ctx), err
	}
	return c.Snapshot(ctx), nil
}

func (c *Controller) ResolvePermission(requestID, optionID string) (Snapshot, error) {
	c.mu.Lock()
	ch := c.pending[requestID]
	if ch != nil {
		delete(c.pending, requestID)
	}
	c.mu.Unlock()
	if ch == nil {
		return c.Snapshot(context.Background()), errors.New("permission request is no longer active")
	}
	ch <- permissionChoice{optionID: optionID}
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventPermissionResolved, RequestID: requestID})
	return c.Snapshot(context.Background()), nil
}

func (c *Controller) Snapshot(context.Context) Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshotLocked()
}

func (c *Controller) Subscribe(ctx context.Context) (<-chan Snapshot, func()) {
	updates := make(chan Snapshot, 8)
	c.mu.Lock()
	select {
	case <-c.closed:
		close(updates)
	default:
		c.subs[updates] = struct{}{}
	}
	c.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			c.mu.Lock()
			if _, ok := c.subs[updates]; ok {
				delete(c.subs, updates)
				close(updates)
			}
			c.mu.Unlock()
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			unsubscribe()
		case <-c.closed:
		}
	}()
	return updates, unsubscribe
}

func (c *Controller) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.mu.Lock()
		if c.cancel != nil {
			c.cancel()
		}
		client := c.client
		for updates := range c.subs {
			close(updates)
			delete(c.subs, updates)
		}
		c.mu.Unlock()
		if client != nil {
			_ = client.Close()
		}
	})
}

func (c *Controller) refreshSessions(ctx context.Context, workspace string) error {
	client, err := c.connectedClient()
	if err != nil {
		return err
	}
	var result sessionListResult
	if err := client.Call(ctx, "session/list", map[string]string{"cwd": workspace}, &result); err != nil {
		return err
	}
	sessions := make([]featuredesktop.SessionState, 0, len(result.Sessions))
	for _, item := range result.Sessions {
		sessions = append(sessions, featuredesktop.SessionState{ID: item.SessionID, Title: item.Title, Workspace: item.Cwd, WorkspaceKey: item.WorkspaceKey, WorkspaceName: item.WorkspaceName, Status: featuredesktop.TaskIdle})
	}
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionsReplaced, Sessions: sessions})
	return nil
}

func (c *Controller) refreshSessionMetadata(ctx context.Context, sessionID string) {
	client, err := c.connectedClient()
	if err != nil {
		return
	}
	var runtime runtimeResult
	if err := client.Call(ctx, "protonman/session/runtime", map[string]string{"sessionId": sessionID}, &runtime); err == nil {
		c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionRuntimeUpdated, SessionID: sessionID, Runtime: featuredesktop.RuntimeSettingsState{Provider: runtime.Provider, Model: runtime.Model, Reasoning: runtime.Reasoning, LowConcurrency: runtime.LowConcurrency}})
	}
	var contextState contextResult
	if err := client.Call(ctx, "protonman/session/context", map[string]string{"sessionId": sessionID}, &contextState); err == nil {
		items := make([]featuredesktop.TodoItemState, 0, len(contextState.Todo.Items))
		for _, item := range contextState.Todo.Items {
			items = append(items, featuredesktop.TodoItemState{ID: item.ID, Text: item.Text, Status: item.Status})
		}
		c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionContextUpdated, SessionID: sessionID, Context: featuredesktop.SessionContextState{Goal: contextState.Goal, Todo: featuredesktop.TodoState{Revision: contextState.Todo.Revision, Items: items}}})
	}
	var memory memoryResult
	if err := client.Call(ctx, "protonman/session/memory", map[string]string{"sessionId": sessionID}, &memory); err == nil {
		project := func(entries []memoryEntryResult) []featuredesktop.MemoryEntryState {
			result := make([]featuredesktop.MemoryEntryState, 0, len(entries))
			for _, entry := range entries {
				result = append(result, featuredesktop.MemoryEntryState{ID: entry.ID, Scope: entry.Scope, Kind: entry.Kind, Key: entry.Key, Value: entry.Value, Confidence: entry.Confidence, UsageCount: entry.UsageCount})
			}
			return result
		}
		c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionMemoryUpdated, SessionID: sessionID, Memory: featuredesktop.MemoryState{WorkspaceKey: memory.WorkspaceKey, Workspace: project(memory.Workspace), Global: project(memory.Global)}})
	}
}

func (c *Controller) connectedClient() (*acpclient.Client, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()
	if client == nil {
		return nil, ErrNotConnected
	}
	return client, nil
}

func (c *Controller) watchClient(client *acpclient.Client) {
	<-client.Done()
	c.mu.Lock()
	if c.client == client {
		c.client = nil
		c.connection = "disconnected"
		c.status = "Protonman disconnected"
		c.state = featuredesktop.Reduce(c.state, featuredesktop.Event{Kind: featuredesktop.EventPermissionsCleared})
	}
	c.mu.Unlock()
	c.publish()
}

func (c *Controller) handleEvent(event acpclient.Event) {
	if event.Method != "session/update" {
		return
	}
	var update sessionUpdate
	if json.Unmarshal(event.Params, &update) != nil {
		return
	}
	c.mu.Lock()
	if c.loadingSessionID != "" && c.loadingSessionID == update.SessionID {
		c.state = featuredesktop.Reduce(c.state, featuredesktop.Event{Kind: featuredesktop.EventSessionTimelineReset, SessionID: update.SessionID})
		c.loadingSessionID = ""
	}
	c.mu.Unlock()
	normalized := featuredesktop.SessionUpdate{
		SessionID:  update.SessionID,
		Kind:       update.Update.Kind,
		ToolCallID: update.Update.ToolCallID,
		Title:      update.Update.Title,
		Status:     update.Update.Status,
		Text:       contentText(update.Update.Content),
	}
	if update.Update.Kind == "protonman_subagent_update" {
		c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSubagentUpserted, SessionID: update.SessionID, Subagent: featuredesktop.SubagentState{ID: update.Update.AgentID, Profile: update.Update.Profile, Task: update.Update.Task, Status: update.Update.Status, Summary: update.Update.Summary}})
		return
	}
	if update.Update.Kind == "current_mode_update" {
		c.reduce(featuredesktop.Event{Kind: featuredesktop.EventSessionModeUpdated, SessionID: update.SessionID, Mode: update.Update.ModeID})
		return
	}
	if reducerEvent, ok := featuredesktop.TimelineEvent(normalized); ok {
		c.reduce(reducerEvent)
	}
}

func (c *Controller) abortConnection(client *acpclient.Client) {
	c.mu.Lock()
	if c.client == client {
		c.client = nil
	}
	c.mu.Unlock()
	_ = client.Close()
}

func (c *Controller) handleRequest(ctx context.Context, request acpclient.Request) (any, error) {
	if request.Method != "session/request_permission" {
		return nil, acpclient.ErrMethodNotHandled
	}
	var params permissionParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return nil, err
	}
	requestID := strings.Trim(string(request.ID), `"`)
	options := make([]featuredesktop.PermissionOption, 0, len(params.Options))
	for _, option := range params.Options {
		options = append(options, featuredesktop.PermissionOption{ID: option.ID, Name: option.Name, Kind: option.Kind})
	}
	title := "Permission required"
	if toolName, ok := params.ToolCall["title"].(string); ok && strings.TrimSpace(toolName) != "" {
		title = toolName
	}
	choice := make(chan permissionChoice, 1)
	c.mu.Lock()
	c.pending[requestID] = choice
	c.mu.Unlock()
	c.reduce(featuredesktop.Event{Kind: featuredesktop.EventPermissionRequested, SessionID: params.SessionID, Permission: featuredesktop.PermissionRequest{RequestID: requestID, SessionID: params.SessionID, Title: title, Options: options}})
	defer func() {
		c.mu.Lock()
		delete(c.pending, requestID)
		c.mu.Unlock()
	}()
	select {
	case selected := <-choice:
		return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": selected.optionID}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, context.Canceled
	}
}

func (c *Controller) reduce(event featuredesktop.Event) {
	c.mu.Lock()
	c.state = featuredesktop.Reduce(c.state, event)
	c.mu.Unlock()
	c.publish()
}

func (c *Controller) snapshotLocked() Snapshot {
	snapshot := Snapshot{Status: c.status, Connection: c.connection, ActiveSessionID: c.state.ActiveSessionID}
	snapshot.Sessions = make([]SessionSnapshot, 0, len(c.state.Sessions))
	for _, session := range c.state.Sessions {
		view := SessionSnapshot{ID: session.ID, Title: session.Title, Workspace: session.Workspace, Mode: session.Mode, Status: string(session.Status), Timeline: make([]TimelineView, 0, len(session.Timeline)), ProviderOptions: cloneModelOptions(c.providerOptions[session.ID]), ModelOptions: cloneModelOptions(c.modelOptions[session.ID]), ModelOptionsError: c.modelOptionsError[session.ID], Runtime: RuntimeView{Provider: session.Runtime.Provider, Model: session.Runtime.Model, Reasoning: session.Runtime.Reasoning, LowConcurrency: session.Runtime.LowConcurrency}, Context: ContextView{Goal: session.Context.Goal, Todo: TodoView{Revision: session.Context.Todo.Revision}, Memory: MemoryView{WorkspaceKey: session.Context.Memory.WorkspaceKey}}}
		for _, item := range session.Timeline {
			view.Timeline = append(view.Timeline, TimelineView{Kind: string(item.Kind), ID: item.ID, Title: item.Title, Text: item.Text, Status: item.Status})
		}
		for _, item := range session.Subagents {
			view.Subagents = append(view.Subagents, SubagentView{ID: item.ID, Profile: item.Profile, Task: item.Task, Summary: item.Summary, Status: item.Status})
		}
		for _, item := range session.Context.Todo.Items {
			view.Context.Todo.Items = append(view.Context.Todo.Items, TodoItemView{ID: item.ID, Text: item.Text, Status: item.Status})
		}
		view.Context.Memory.Workspace = memoryViews(session.Context.Memory.Workspace)
		view.Context.Memory.Global = memoryViews(session.Context.Memory.Global)
		snapshot.Sessions = append(snapshot.Sessions, view)
	}
	snapshot.PermissionInbox = make([]PermissionView, 0, len(c.state.PermissionInbox))
	for _, item := range c.state.PermissionInbox {
		view := PermissionView{RequestID: item.RequestID, SessionID: item.SessionID, Title: item.Title, Detail: item.Detail}
		for _, option := range item.Options {
			view.Options = append(view.Options, PermissionViewOption{ID: option.ID, Name: option.Name, Kind: option.Kind})
		}
		snapshot.PermissionInbox = append(snapshot.PermissionInbox, view)
	}
	return snapshot
}

func memoryViews(items []featuredesktop.MemoryEntryState) []MemoryEntryView {
	result := make([]MemoryEntryView, 0, len(items))
	for _, item := range items {
		result = append(result, MemoryEntryView{ID: item.ID, Scope: item.Scope, Kind: item.Kind, Key: item.Key, Value: item.Value, Confidence: item.Confidence, UsageCount: item.UsageCount})
	}
	return result
}

func modelOptionsFromConfig(options []sessionConfigOption) []ModelOptionView {
	for _, option := range options {
		if option.ID == "model" {
			return cloneModelOptions(option.Options)
		}
	}
	return []ModelOptionView{}
}

func providerOptionsFromConfig(options []sessionConfigOption) []ModelOptionView {
	for _, option := range options {
		if option.ID == "provider" {
			return cloneModelOptions(option.Options)
		}
	}
	return []ModelOptionView{}
}

func (c *Controller) storeConfigOptions(sessionID string, options []sessionConfigOption) {
	c.mu.Lock()
	c.storeConfigOptionsLocked(sessionID, options)
	c.mu.Unlock()
	c.publish()
}

func (c *Controller) storeConfigOptionsLocked(sessionID string, options []sessionConfigOption) {
	c.providerOptions[sessionID] = providerOptionsFromConfig(options)
	c.modelOptions[sessionID] = modelOptionsFromConfig(options)
	c.modelOptionsError[sessionID] = modelOptionsErrorFromConfig(options)
}

func modelOptionsErrorFromConfig(options []sessionConfigOption) string {
	for _, option := range options {
		if option.ID == "model" {
			return option.Error
		}
	}
	return ""
}

func cloneModelOptions(options []ModelOptionView) []ModelOptionView {
	result := make([]ModelOptionView, len(options))
	copy(result, options)
	return result
}

func (c *Controller) publish() {
	c.mu.RLock()
	snapshot := c.snapshotLocked()
	for sub := range c.subs {
		select {
		case sub <- snapshot:
		default:
		}
	}
	c.mu.RUnlock()
}

func (c *Controller) updateStatus(status, connection string) Snapshot {
	c.mu.Lock()
	c.status = status
	if connection != "" {
		c.connection = connection
	}
	c.mu.Unlock()
	c.publish()
	return c.Snapshot(context.Background())
}

func (c *Controller) nextID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, c.requestSeq.Add(1))
}

func normalizeWorkspace(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = defaultWorkspace
	}
	if absolute, err := filepath.Abs(workspace); err == nil {
		return absolute
	}
	return workspace
}

func resolveBinary() string {
	if value := strings.TrimSpace(os.Getenv("PROTONMAN_BINARY")); value != "" {
		return value
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		for _, candidate := range []string{filepath.Join(dir, "protonman"), filepath.Join(dir, "libexec", "protonman")} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	if path, err := exec.LookPath("protonman"); err == nil {
		return path
	}
	return ""
}

func appendSession(sessions []featuredesktop.SessionState, session featuredesktop.SessionState) []featuredesktop.SessionState {
	for i := range sessions {
		if sessions[i].ID == session.ID {
			sessions[i] = session
			return sessions
		}
	}
	return append(sessions, session)
}

func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var block struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &block) == nil && block.Text != "" {
		return block.Text
	}
	var blocks []struct {
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, item := range blocks {
			if item.Content.Text != "" {
				parts = append(parts, item.Content.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
