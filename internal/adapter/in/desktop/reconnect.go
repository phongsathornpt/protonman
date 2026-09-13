//go:build desktop

package desktop

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	reconnectInitialDelay = time.Second
	reconnectMaxDelay     = 8 * time.Second
)

func (a *application) superviseConnection() {
	binary := strings.TrimSpace(os.Getenv("PROTONMAN_BINARY"))
	if binary == "" {
		binary = "protonman"
	}

	delay := reconnectInitialDelay
	for {
		if a.ctx.Err() != nil {
			return
		}
		client, err := acpclient.Start(a.ctx, binary, a.handleEvent)
		if err != nil {
			a.setStatus("Disconnected · retrying · " + err.Error())
			if !waitReconnect(a.ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}
		client.SetRequestHandler(a.handleRequest)
		if err := a.initializeClient(client); err != nil {
			_ = client.Close()
			a.setStatus("ACP reconnect failed · " + err.Error())
			if !waitReconnect(a.ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}

		a.setClient(client)
		delay = reconnectInitialDelay
		a.resumeKnownSessions(client)
		a.refreshSessions()

		select {
		case <-a.ctx.Done():
			_ = client.Close()
			return
		case <-client.Done():
			a.markDisconnected(client)
		}
	}
}

func (a *application) initializeClient(client *acpclient.Client) error {
	var result struct {
		ProtocolVersion int `json:"protocolVersion"`
		AgentInfo       struct {
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	if err := client.Call(a.ctx, "initialize", map[string]any{
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

func (a *application) resumeKnownSessions(client *acpclient.Client) {
	a.mu.Lock()
	sessions := append([]desktopstate.SessionState(nil), a.state.Sessions...)
	a.mu.Unlock()
	for _, session := range sessions {
		if strings.TrimSpace(session.ID) == "" {
			continue
		}
		_ = client.Call(a.ctx, "session/resume", map[string]any{
			"sessionId": session.ID,
			"cwd":       session.Workspace,
		}, nil)
	}
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
