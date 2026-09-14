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
	reconnectInitialDelay  = time.Second
	reconnectMaxDelay      = 8 * time.Second
	reconnectRequestTimeout = 15 * time.Second
)

func (a *application) superviseConnection() {
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	if a.desktopApp != nil {
		a.desktopApp.Lifecycle().SetOnStopped(cancel)
	}
	binary := resolveACPBinary()

	delay := reconnectInitialDelay
	for {
		if ctx.Err() != nil {
			return
		}
		client, err := acpclient.Start(ctx, binary, a.handleEvent)
		if err != nil {
			a.setStatus("Disconnected · retrying · " + err.Error())
			if !waitReconnect(ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}
		client.SetRequestHandler(a.handleRequest)
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

		a.setClient(client)
		delay = reconnectInitialDelay
		a.resumeKnownSessions(ctx, client)
		go a.refreshSessions()

		select {
		case <-ctx.Done():
			_ = client.Close()
			return
		case <-client.Done():
			if ctx.Err() != nil {
				return
			}
			a.markDisconnected(client)
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
	a.setStatus("Protonman " + version + " · ACP v1")
	return nil
}

func (a *application) resumeKnownSessions(ctx context.Context, client *acpclient.Client) {
	a.mu.Lock()
	sessions := append([]desktopstate.SessionState(nil), a.state.Sessions...)
	a.mu.Unlock()
	mcpServers := a.mcpServersPayload()
	for _, session := range sessions {
		if strings.TrimSpace(session.ID) == "" {
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

func (a *application) markDisconnected(client *acpclient.Client) {
	a.mu.Lock()
	if a.client != client {
		a.mu.Unlock()
		return
	}
	a.client = nil
	a.state = desktopstate.MarkDisconnected(a.state)
	a.permissionWaiters = make(map[string]chan string)
	a.mu.Unlock()
	a.setStatus("Disconnected · reconnecting…")
	fyne.Do(func() { a.list.Refresh() })
	a.refreshActiveView()
	a.refreshPermissionView()
}

func (a *application) setClient(client *acpclient.Client) {
	a.mu.Lock()
	a.client = client
	a.mu.Unlock()
}

func (a *application) currentClient() *acpclient.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client
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
	return a.client == client
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
