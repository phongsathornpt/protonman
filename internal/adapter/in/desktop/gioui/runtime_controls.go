//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func (c *controller) setRuntimeModel(provider, model string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	c.mu.Lock()
	sessionID := strings.TrimSpace(c.state.ActiveSessionID)
	session, ok := desktopSessionByID(c.state, sessionID, c.state.ActiveAgentID)
	agentID := controllerAgentID
	if ok && strings.TrimSpace(session.AgentID) != "" {
		agentID = strings.TrimSpace(session.AgentID)
	}
	if c.agentDefaultModels == nil {
		c.agentDefaultModels = make(map[string]string)
	}
	c.agentDefaultModels[agentID] = model
	if c.preferences != nil {
		go func(aID, m string) {
			_ = c.preferences.SetAgentDefaultModel(c.ctx, aID, m)
		}(agentID, model)
	}
	if ok && session.AgentID != "" && session.AgentID != controllerAgentID {
		desktopstate.Apply(&c.state, desktopstate.Event{
			Kind:      desktopstate.EventSessionRuntimeUpdated,
			AgentID:   session.AgentID,
			SessionID: sessionID,
			Runtime: desktopstate.RuntimeSettingsState{
				Provider:       provider,
				Model:          model,
				Reasoning:      session.Runtime.Reasoning,
				LowConcurrency: session.Runtime.LowConcurrency,
				PermissionMode: session.Runtime.PermissionMode,
			},
		})
		c.revision++
		client := c.clients[agentID]
		c.mu.Unlock()
		c.notify()
		if client != nil {
			go func() {
				callCtx, cancel := context.WithTimeout(c.ctx, reconnectRequestTimeout)
				defer cancel()
				var result struct {
					ConfigOptions []acpConfigOption `json:"configOptions,omitempty"`
					Models        *acpModelsResult  `json:"models,omitempty"`
				}
				err := client.Call(callCtx, "session/set_config_option", map[string]any{
					"sessionId": sessionID,
					"configId":  "model",
					"value":     model,
				}, &result)
				if isACPMethodNotFound(err) {
					err = client.Call(callCtx, "session/set_model", map[string]any{
						"sessionId": sessionID,
						"modelId":   model,
					}, &result)
				}
				if err == nil {
					if models, currentModel := extractModelsFromACP(result.ConfigOptions, result.Models); len(models) > 0 {
						c.mu.Lock()
						c.setAgentAvailableModelsLocked(agentID, models)
						if sess := desktopstateSessionPointer(&c.state, sessionID, agentID); sess != nil {
							sess.AvailableModels = models
							if currentModel != "" {
								sess.Runtime.Model = currentModel
							}
							c.revision++
						}
						c.mu.Unlock()
						c.notify()
					}
				}
			}()
		}
		return
	}
	if provider == "" && ok {
		provider = session.Runtime.Provider
	}
	c.mu.Unlock()

	params := map[string]any{
		"model": model,
	}
	if provider != "" {
		params["provider"] = provider
	}
	c.startRuntimeMutation("protonman/session/set_model", params)
}

func (c *controller) setRuntimeReasoning(value string) {
	c.setRuntimeChoice("protonman/session/set_reasoning", "reasoning", value)
}

func (c *controller) setRuntimeLowConcurrency(value string) {
	c.setRuntimeChoice("protonman/session/set_low_concurrency", "lowConcurrency", value)
}

func (c *controller) setRuntimePermissionMode(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	c.mu.Lock()
	sessionID := strings.TrimSpace(c.state.ActiveSessionID)
	session, ok := desktopSessionByID(c.state, sessionID, c.state.ActiveAgentID)
	if ok && session.AgentID != "" && session.AgentID != controllerAgentID {
		agentID := session.AgentID
		client := c.clients[agentID]
		profile := c.profiles[agentID]
		if isClineACPProfile(profile) {
			if client == nil || c.connections[agentID] != connectionConnected || sessionBusy(session.Status) || c.runtimeMutation != "" {
				c.mu.Unlock()
				return
			}
			c.runtimeMutation = sessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: sessionID})
			c.statuses[agentID] = "Updating permission mode…"
			c.revision++
			c.mu.Unlock()
			c.notify()
			go c.updateClinePermissionMode(client, agentID, sessionID, value)
			return
		}
		if sess := desktopstateSessionPointer(&c.state, sessionID, session.AgentID); sess != nil {
			sess.Runtime.PermissionMode = value
			c.revision++
		}
		c.mu.Unlock()
		c.notify()
		if client != nil {
			go func() {
				callCtx, cancel := context.WithTimeout(c.ctx, reconnectRequestTimeout)
				defer cancel()
				var result struct{}
				_ = client.Call(callCtx, "session/set_mode", map[string]any{
					"sessionId": sessionID,
					"modeId":    value,
				}, &result)
			}()
		}
		return
	}
	c.mu.Unlock()
	c.setRuntimeChoice("protonman/session/set_permission_mode", "permissionMode", value)
}

func (c *controller) updateClinePermissionMode(client *acpclient.Client, agentID, sessionID, value string) {
	defer c.finishRuntimeMutation(client, sessionID)
	callCtx, cancel := context.WithTimeout(c.ctx, reconnectRequestTimeout)
	defer cancel()
	if err := setClinePermissionMode(callCtx, client, sessionID, value); err != nil {
		if c.ctx == nil || c.ctx.Err() == nil {
			c.setRuntimeStatus(client, sessionID, "Permission mode update failed · "+compactError(err))
		}
		return
	}

	c.mu.Lock()
	session, ok := desktopSessionByID(c.state, sessionID, agentID)
	if !ok || session.AgentID != agentID || !c.clientCurrentLocked(agentID, client) {
		c.mu.Unlock()
		return
	}
	if current := desktopstateSessionPointer(&c.state, sessionID, agentID); current != nil {
		current.Runtime.PermissionMode = value
		c.revision++
	}
	c.statuses[agentID] = "Permission mode updated"
	c.mu.Unlock()
	c.notify()
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
	session, ok := desktopSessionByID(c.state, sessionID, c.state.ActiveAgentID)
	client, agentID := c.clientForSessionLocked(sessionID, c.state.ActiveAgentID)
	if !ok || (session.AgentID != "" && session.AgentID != controllerAgentID) || sessionBusy(session.Status) || client == nil || c.connections[agentID] != connectionConnected || c.runtimeMutation != "" {
		c.mu.Unlock()
		return
	}
	params := make(map[string]any, len(values)+1)
	for key, value := range values {
		params[key] = value
	}
	params["sessionId"] = sessionID
	mutationKey := sessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: sessionID})
	c.runtimeMutation = mutationKey
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
			c.refreshSessionRuntime(sessionID, true, agentID)
			return
		}
		if !c.applySessionRuntimeForAgent(client, result, agentID) {
			c.setRuntimeStatus(client, sessionID, "Runtime update returned an invalid session result")
			return
		}
		c.setRuntimeStatus(client, sessionID, "Runtime updated")
	}()
}

func (c *controller) finishRuntimeMutation(client *acpclient.Client, sessionID string) {
	c.mu.Lock()
	agentID := c.agentIDForClientLocked(client)
	mutationKey := sessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: sessionID})
	if c.runtimeMutation == mutationKey || c.runtimeMutation == sessionID {
		c.runtimeMutation = ""
		c.revision++
	}
	c.mu.Unlock()
	c.notify()
}

func (c *controller) setRuntimeStatus(client *acpclient.Client, sessionID, status string) {
	c.mu.Lock()
	agentID := c.agentIDForClientLocked(client)
	session, ok := desktopSessionByID(c.state, sessionID, agentID)
	if !ok || !c.clientCurrentLocked(session.AgentID, client) {
		c.mu.Unlock()
		return
	}
	agentID = session.AgentID
	if agentID == "" {
		agentID = c.activeAgentID
	}
	if agentID == "" {
		agentID = controllerAgentID
	}
	c.statuses[agentID] = status
	c.revision++
	c.mu.Unlock()
	c.notify()
}
