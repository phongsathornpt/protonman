//go:build desktop || desktop_gio

package gioui

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
)

func (c *controller) setRuntimeModel(provider, model string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return
	}
	c.startRuntimeMutation("protonman/session/set_model", map[string]any{
		"provider": provider,
		"model":    model,
	})
}

func (c *controller) setRuntimeReasoning(value string) {
	c.setRuntimeChoice("protonman/session/set_reasoning", "reasoning", value)
}

func (c *controller) setRuntimeLowConcurrency(value string) {
	c.setRuntimeChoice("protonman/session/set_low_concurrency", "lowConcurrency", value)
}

func (c *controller) setRuntimeChoice(method, field, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	c.startRuntimeMutation(method, map[string]any{field: value})
}

func (c *controller) startRuntimeMutation(method string, values map[string]any) {
	c.mu.Lock()
	sessionID := strings.TrimSpace(c.state.ActiveSessionID)
	session, ok := desktopSessionByID(c.state, sessionID)
	client, agentID := c.clientForSessionLocked(sessionID)
	if !ok || session.AgentID != controllerAgentID || sessionBusy(session.Status) || client == nil || c.connections[agentID] != connectionConnected || c.runtimeMutation != "" {
		c.mu.Unlock()
		return
	}
	params := make(map[string]any, len(values)+1)
	for key, value := range values {
		params[key] = value
	}
	params["sessionId"] = sessionID
	c.runtimeMutation = sessionID
	c.statuses[agentID] = "Updating runtime…"
	c.revision++
	c.mu.Unlock()
	c.notify()

	go func() {
		defer c.finishRuntimeMutation(client, sessionID)
		var result sessionRuntimeResult
		if err := c.callInspector(client, method, params, &result); err != nil {
			if c.ctx == nil || c.ctx.Err() == nil {
				c.setRuntimeStatus(client, sessionID, "Runtime update failed · "+compactError(err))
			}
			c.refreshSessionRuntime(sessionID, true)
			return
		}
		if !c.applySessionRuntime(client, result) {
			c.setRuntimeStatus(client, sessionID, "Runtime update returned an invalid session result")
			return
		}
		c.setRuntimeStatus(client, sessionID, "Runtime updated")
	}()
}

func (c *controller) finishRuntimeMutation(client *acpclient.Client, sessionID string) {
	c.mu.Lock()
	session, ok := desktopSessionByID(c.state, sessionID)
	if !ok || !c.clientCurrentLocked(session.AgentID, client) || c.runtimeMutation != sessionID {
		c.mu.Unlock()
		return
	}
	c.runtimeMutation = ""
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) setRuntimeStatus(client *acpclient.Client, sessionID, status string) {
	c.mu.Lock()
	session, ok := desktopSessionByID(c.state, sessionID)
	if !ok || !c.clientCurrentLocked(session.AgentID, client) {
		c.mu.Unlock()
		return
	}
	c.statuses[session.AgentID] = status
	c.revision++
	c.mu.Unlock()
	c.notify()
}
