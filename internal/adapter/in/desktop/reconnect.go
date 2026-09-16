//go:build desktop

package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	reconnectInitialDelay   = time.Second
	reconnectMaxDelay       = 8 * time.Second
	reconnectRequestTimeout = 15 * time.Second
)

func (a *application) superviseConnection() {
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	if a.desktopApp != nil {
		a.desktopApp.Lifecycle().SetOnStopped(cancel)
	}
	a.mu.Lock()
	profiles := make([]agentProfile, 0, len(a.profiles))
	for _, profile := range a.profiles {
		profiles = append(profiles, profile)
	}
	a.mu.Unlock()
	for _, profile := range profiles {
		go a.superviseAgentConnection(ctx, profile)
	}
	<-ctx.Done()
}

func (a *application) superviseAgentConnection(ctx context.Context, profile agentProfile) {
	delay := reconnectInitialDelay
	for {
		if ctx.Err() != nil {
			return
		}
		client, err := acpclient.StartCommand(ctx, profile.Command, func(event acpclient.Event) {
			a.handleAgentEvent(profile.ID, event)
		})
		if err != nil {
			a.setStatus("Disconnected · retrying · " + err.Error())
			if !waitReconnect(ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}
		client.SetRequestHandler(func(requestCtx context.Context, request acpclient.Request) (any, error) {
			return a.handleAgentRequest(profile.ID, requestCtx, request)
		})
		if err := a.initializeClient(ctx, client); err != nil {
			_ = client.Close()
			if ctx.Err() != nil {
				return
			}
			a.setStatus("ACP reconnect failed · " + err.Error())
			if !waitReconnect(ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}

		a.setClient(profile.ID, client)
		delay = reconnectInitialDelay
		a.resumeKnownSessions(ctx, profile.ID, client)
		go a.refreshSessions()

		select {
		case <-ctx.Done():
			_ = client.Close()
			return
		case <-client.Done():
			if ctx.Err() != nil {
				return
			}
			a.markDisconnected(profile.ID, client)
		}
	}
}

func resolveACPBinary() string {
	executable, _ := os.Executable()
	return resolveACPBinaryFor(os.Getenv("PROTONMAN_BINARY"), executable, func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	})
}

func resolveACPBinaryFor(override, executable string, isFile func(string) bool) string {
	if override = strings.TrimSpace(override); override != "" {
		return override
	}
	if executable = strings.TrimSpace(executable); executable != "" && isFile != nil {
		candidate := filepath.Join(filepath.Dir(executable), "libexec", "protonman")
		if isFile(candidate) {
			return candidate
		}
	}
	return "protonman"
}

func (a *application) initializeClient(ctx context.Context, client *acpclient.Client) error {
	var result struct {
		ProtocolVersion int `json:"protocolVersion"`
		AgentInfo       struct {
			Name    string `json:"name"`
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	if err := callReconnectRPC(ctx, client, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientInfo":         map[string]any{"name": "protonman-desktop", "title": "Protonman Desktop"},
		"clientCapabilities": map[string]any{},
	}, &result); err != nil {
		return err
	}
	if result.ProtocolVersion != 1 {
		return fmt.Errorf("unsupported ACP v%d", result.ProtocolVersion)
	}
	version := strings.TrimPrefix(result.AgentInfo.Version, "v")
	if version == "" {
		version = "connected"
	}
	agentName := strings.TrimSpace(result.AgentInfo.Title)
	if agentName == "" {
		agentName = strings.TrimSpace(result.AgentInfo.Name)
	}
	if agentName == "" {
		a.mu.Lock()
		agentName = a.agent.DisplayName
		a.mu.Unlock()
	}
	if agentName == "" {
		agentName = "ACP agent"
	}
	a.setStatus(agentName + " " + version + " · ACP v1")
	return nil
}

func (a *application) resumeKnownSessions(ctx context.Context, agentID string, client *acpclient.Client) {
	a.mu.Lock()
	sessions := append([]desktopstate.SessionState(nil), a.state.Sessions...)
	a.mu.Unlock()
	mcpServers := a.mcpServersPayload()
	for _, session := range sessions {
		if strings.TrimSpace(session.ID) == "" {
			continue
		}
		if session.AgentID != "" && session.AgentID != agentID {
			continue
		}
		if session.AgentID == "" && agentID != defaultAgentID {
			continue
		}
		workspace := a.resolveWorkspacePath(session.WorkspaceKey, session.Workspace)
		if workspace == "" {
			a.setStatus("Session resume skipped · workspace path unavailable for " + shortID(session.ID))
			continue
		}
		params := map[string]any{"sessionId": session.ID, "cwd": workspace}
		if len(mcpServers) > 0 {
			params["mcpServers"] = mcpServers
		}
		if err := callReconnectRPC(ctx, client, "session/resume", params, nil); err != nil && ctx.Err() == nil {
			a.setStatus("Session resume failed · " + err.Error())
		}
	}
}

func callReconnectRPC(ctx context.Context, client *acpclient.Client, method string, params any, result any) error {
	callCtx, cancel := context.WithTimeout(ctx, reconnectRequestTimeout)
	defer cancel()
	return client.Call(callCtx, method, params, result)
}

func (a *application) markDisconnected(agentID string, client *acpclient.Client) {
	a.mu.Lock()
	if a.clients[agentID] != client {
		a.mu.Unlock()
		return
	}
	delete(a.clients, agentID)
	if a.activeAgentID == agentID {
		a.client = nil
	}
	a.state = desktopstate.MarkAgentDisconnected(a.state, agentID)
	a.permissionWaiters = make(map[string]chan string)
	a.mu.Unlock()
	resetLoadingSessionHistories(a)
	a.setStatus("Disconnected · reconnecting…")
	fyne.Do(func() { a.list.Refresh() })
	a.refreshActiveView()
	a.refreshPermissionView()
}

func (a *application) setClient(agentID string, client *acpclient.Client) {
	a.mu.Lock()
	if a.clients == nil {
		a.clients = make(map[string]*acpclient.Client)
	}
	a.clients[agentID] = client
	if a.activeAgentID == agentID {
		a.client = client
		a.agent = a.profiles[agentID]
	}
	a.mu.Unlock()
}

func (a *application) currentClient() *acpclient.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeAgentID != "" {
		return a.clients[a.activeAgentID]
	}
	return a.client
}

func (a *application) clientForSession(sessionID string) *acpclient.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.clientForSessionLocked(sessionID)
}

func (a *application) clientForSessionLocked(sessionID string) *acpclient.Client {
	for _, session := range a.state.Sessions {
		if session.ID == sessionID && session.AgentID != "" {
			return a.clients[session.AgentID]
		}
	}
	return a.clients[a.activeAgentID]
}

func (a *application) clientIsCurrent(client *acpclient.Client) bool {
	if client == nil {
		return false
	}
	select {
	case <-client.Done():
		return false
	default:
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, current := range a.clients {
		if current == client {
			return true
		}
	}
	return false
}

func (a *application) handleAgentEvent(agentID string, event acpclient.Event) {
	// ACP session IDs are the routing key for updates. The agent ID is retained
	// by the session record and is used for subsequent client-to-agent calls.
	a.handleEvent(event)
}

func (a *application) handleAgentRequest(_ string, ctx context.Context, request acpclient.Request) (any, error) {
	return a.handleRequest(ctx, request)
}

func waitReconnect(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextReconnectDelay(delay time.Duration) time.Duration {
	delay *= 2
	if delay > reconnectMaxDelay {
		return reconnectMaxDelay
	}
	return delay
}
