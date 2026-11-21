//go:build desktop || desktop_gio

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func (c *controller) SetRuntimeModel(provider, model string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if model == "" {
		c.mu.Lock()
		c.status = "Runtime not updated \u00b7 no model is selected"
		c.revision++
		c.mu.Unlock()
		c.notify()
		return
	}
	c.mu.Lock()
	sessionID := strings.TrimSpace(c.state.ActiveSessionID)
	session, ok := SessionByID(c.state, sessionID, c.state.ActiveAgentID)
	agentID := ProtonmanAgentID
	if ok && strings.TrimSpace(session.AgentID) != "" {
		agentID = strings.TrimSpace(session.AgentID)
	}
	if c.agentDefaultModels == nil {
		c.agentDefaultModels = make(map[string]string)
	}
	c.agentDefaultModels[agentID] = model
	if c.preferences != nil {
		c.persistPreference(func(ctx context.Context) error {
			return c.preferences.SetAgentDefaultModel(ctx, agentID, model)
		})
	}
	if ok && session.AgentID != "" && session.AgentID != ProtonmanAgentID {
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
		client := c.client
		c.mu.Unlock()
		c.notify()
		if client != nil {
			c.spawn(func() {
				callCtx, cancel := context.WithTimeout(c.context(), reconnectRequestTimeout)
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
			})
		}
		return
	}
	if provider == "" && ok {
		provider = session.Runtime.Provider
	}
	currentProvider := ""
	if ok {
		currentProvider = session.Runtime.Provider
	}
	switchesProvider := provider != "" && !equalProviderID(provider, currentProvider)
	c.mu.Unlock()

	if switchesProvider {
		c.setRuntimeModelWithProvider(provider, model)
		return
	}
	params := map[string]any{
		"model": model,
	}
	if provider != "" {
		params["provider"] = provider
	}
	c.startRuntimeMutation("protonman/session/set_model", params)
}

func equalProviderID(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if strings.EqualFold(a, b) {
		return true
	}
	return strings.EqualFold(strings.ReplaceAll(a, "-", ""), strings.ReplaceAll(b, "-", ""))
}

// setRuntimeModelWithProvider switches provider+model through the provider-aware
// legacy runtime method. The standard session/set_config_option fast path only
// carries configId=model, so it cannot move across providers: the backend
// validates the model against the *current* provider's advertised options.
func (c *controller) setRuntimeModelWithProvider(provider, model string) {
	c.mu.Lock()
	sessionID := strings.TrimSpace(c.state.ActiveSessionID)
	session, ok := SessionByID(c.state, sessionID, c.state.ActiveAgentID)
	client, agentID := c.clientForSession(sessionID, c.state.ActiveAgentID)
	if !ok || (session.AgentID != "" && session.AgentID != ProtonmanAgentID) || SessionBusy(session.Status) || client == nil || c.connection != ConnectionConnected || c.runtimeMutation != "" {
		if !ok || client == nil || c.connection != ConnectionConnected {
			reason := runtimeBlockedReason(ok, client, c.connection, session)
			c.status = "Runtime not updated \u00b7 " + reason
			c.revision++
		}
		c.mu.Unlock()
		if ok && client != nil && c.connection != ConnectionConnected {
			c.notify()
		}
		return
	}
	previous := session.Runtime
	if sess := desktopstateSessionPointer(&c.state, sessionID, session.AgentID); sess != nil {
		sess.Runtime.Provider = provider
		sess.Runtime.Model = model
		c.revision++
	}
	params := map[string]any{"sessionId": sessionID, "provider": provider, "model": model}
	mutationKey := SessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: sessionID})
	c.runtimeMutation = mutationKey
	c.status = "Updating runtime\u2026"
	c.revision++
	c.mu.Unlock()
	c.notify()

	c.spawn(func() {
		defer c.finishRuntimeMutation(client, sessionID)
		var result sessionRuntimeResult
		if err := c.callInspector(client, "protonman/session/set_model", params, &result); err != nil {
			c.mu.Lock()
			if sess := desktopstateSessionPointer(&c.state, sessionID, session.AgentID); sess != nil {
				sess.Runtime = previous
				c.revision++
			}
			c.mu.Unlock()
			c.notify()
			if c.context().Err() == nil {
				c.setRuntimeStatus(client, sessionID, "Runtime update failed \u00b7 "+compactError(err))
			}
			c.refreshSessionRuntime(sessionID, true, agentID)
			return
		}
		if !c.applySessionRuntime(client, result, agentID) {
			c.setRuntimeStatus(client, sessionID, "Runtime update returned an invalid session result")
			return
		}
		c.setRuntimeStatus(client, sessionID, "Runtime updated")
	})
}

// runtimeBlockedReason mirrors expandedPromptBlocked for runtime mutations.
func runtimeBlockedReason(ok bool, client *acpclient.Client, connection ConnectionPhase, session desktopstate.SessionState) string {
	switch {
	case !ok:
		return "no session is selected"
	case client == nil:
		return "the ACP runtime is unavailable"
	case connection != ConnectionConnected:
		return "reconnect to the ACP runtime before updating"
	case SessionBusy(session.Status):
		return "wait for the active turn to finish or stop it first"
	default:
		return "another runtime update is in progress"
	}
}

func (c *controller) SetRuntimeReasoning(value string) {
	c.setRuntimeChoice("protonman/session/set_reasoning", "reasoning", value)
}

func (c *controller) SetRuntimeLowConcurrency(value string) {
	c.setRuntimeChoice("protonman/session/set_low_concurrency", "lowConcurrency", value)
}

func (c *controller) SetRuntimePermissionMode(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	c.mu.Lock()
	sessionID := strings.TrimSpace(c.state.ActiveSessionID)
	session, ok := SessionByID(c.state, sessionID, c.state.ActiveAgentID)
	if ok && session.AgentID != "" && session.AgentID != ProtonmanAgentID {
		c.mu.Unlock()
		return
	}
	if ok {
		client := c.client
		if sess := desktopstateSessionPointer(&c.state, sessionID, session.AgentID); sess != nil {
			sess.Runtime.PermissionMode = value
			c.revision++
		}
		c.mu.Unlock()
		c.notify()
		if client != nil {
			c.spawn(func() {
				callCtx, cancel := context.WithTimeout(c.context(), reconnectRequestTimeout)
				defer cancel()
				var result struct{}
				_ = client.Call(callCtx, "session/set_mode", map[string]any{
					"sessionId": sessionID,
					"modeId":    value,
				}, &result)
			})
		}
		return
	}
	c.mu.Unlock()
	c.setRuntimeChoice("protonman/session/set_permission_mode", "permissionMode", value)
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
	session, ok := SessionByID(c.state, sessionID, c.state.ActiveAgentID)
	client, agentID := c.clientForSession(sessionID, c.state.ActiveAgentID)
	if !ok || (session.AgentID != "" && session.AgentID != ProtonmanAgentID) || SessionBusy(session.Status) || client == nil || c.connection != ConnectionConnected || c.runtimeMutation != "" {
		if session.AgentID == "" || session.AgentID == ProtonmanAgentID {
			reason := runtimeBlockedReason(ok, client, c.connection, session)
			c.status = "Runtime not updated \u00b7 " + reason
			c.revision++
		}
		c.mu.Unlock()
		c.notify()
		return
	}
	params := make(map[string]any, len(values)+1)
	for key, value := range values {
		params[key] = value
	}
	params["sessionId"] = sessionID
	mutationKey := SessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: sessionID})
	c.runtimeMutation = mutationKey
	c.status = "Updating runtime…"
	c.revision++
	c.mu.Unlock()
	c.notify()

	c.spawn(func() {
		defer c.finishRuntimeMutation(client, sessionID)

		var configID string
		var configVal string
		switch method {
		case "protonman/session/set_permission_mode":
			configID = "permissionMode"
			configVal = fmt.Sprint(values["permissionMode"])
		case "protonman/session/set_reasoning":
			configID = "reasoning"
			configVal = fmt.Sprint(values["reasoning"])
		case "protonman/session/set_model":
			configID = "model"
			configVal = fmt.Sprint(values["model"])
		}

		if configID != "" {
			callCtx, cancel := context.WithTimeout(c.context(), reconnectRequestTimeout)
			defer cancel()
			var setOptionResult struct {
				ConfigOptions []acpConfigOption `json:"configOptions"`
			}
			err := client.Call(callCtx, "session/set_config_option", map[string]any{
				"sessionId": sessionID,
				"configId":  configID,
				"value":     configVal,
			}, &setOptionResult)
			if err == nil {
				c.refreshSessionRuntime(sessionID, false, agentID)
				c.setRuntimeStatus(client, sessionID, "Runtime updated")
				return
			}
			if !isACPMethodNotFound(err) {
				if c.context().Err() == nil {
					c.setRuntimeStatus(client, sessionID, "Runtime update failed · "+compactError(err))
				}
				c.refreshSessionRuntime(sessionID, true, agentID)
				return
			}
		}

		var result sessionRuntimeResult
		if err := c.callInspector(client, method, params, &result); err != nil {
			if c.context().Err() == nil {
				c.setRuntimeStatus(client, sessionID, "Runtime update failed · "+compactError(err))
			}
			c.refreshSessionRuntime(sessionID, true, agentID)
			return
		}
		if !c.applySessionRuntime(client, result, agentID) {
			c.setRuntimeStatus(client, sessionID, "Runtime update returned an invalid session result")
			return
		}
		c.setRuntimeStatus(client, sessionID, "Runtime updated")
	})
}

func (c *controller) finishRuntimeMutation(client *acpclient.Client, sessionID string) {
	c.mu.Lock()
	agentID := protonAgentIDForClient(c.client, client)
	mutationKey := SessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: sessionID})
	if c.runtimeMutation == mutationKey || c.runtimeMutation == sessionID {
		c.runtimeMutation = ""
		c.revision++
	}
	c.mu.Unlock()
	c.notify()
}

func (c *controller) setRuntimeStatus(client *acpclient.Client, sessionID, status string) {
	c.mu.Lock()
	agentID := protonAgentIDForClient(c.client, client)
	session, ok := SessionByID(c.state, sessionID, agentID)
	if !ok || !c.isCurrentClient(client) {
		c.mu.Unlock()
		return
	}
	agentID = session.AgentID
	if agentID == "" {
		agentID = ProtonmanAgentID
	}
	c.status = status
	c.revision++
	c.mu.Unlock()
	c.notify()
}
