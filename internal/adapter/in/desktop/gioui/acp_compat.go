//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
)

const acpErrorAuthRequired = -32000

type acpAuthMethod struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type acpAgentFeatures struct {
	AuthMethods                   []acpAuthMethod
	SupportsAdditionalDirectories bool
	SupportsSessionList           bool
	SupportsSessionResume         bool
}

type initializeACPResponse struct {
	ProtocolVersion   int `json:"protocolVersion"`
	AgentCapabilities struct {
		SessionCapabilities struct {
			AdditionalDirectories json.RawMessage `json:"additionalDirectories"`
			List                  json.RawMessage `json:"list"`
			Resume                json.RawMessage `json:"resume"`
		} `json:"sessionCapabilities"`
	} `json:"agentCapabilities"`
	AuthMethods []acpAuthMethod `json:"authMethods"`
}

func (r initializeACPResponse) features() acpAgentFeatures {
	return acpAgentFeatures{
		AuthMethods:                   slices.Clone(r.AuthMethods),
		SupportsAdditionalDirectories: acpCapabilityObjectAdvertised(r.AgentCapabilities.SessionCapabilities.AdditionalDirectories),
		SupportsSessionList:           acpCapabilityObjectAdvertised(r.AgentCapabilities.SessionCapabilities.List),
		SupportsSessionResume:         acpCapabilityObjectAdvertised(r.AgentCapabilities.SessionCapabilities.Resume),
	}
}

func acpCapabilityObjectAdvertised(raw json.RawMessage) bool {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return false
	}
	var capability map[string]json.RawMessage
	return json.Unmarshal(raw, &capability) == nil && capability != nil
}

func (c *controller) setAgentFeatures(agentID string, features acpAgentFeatures) {
	c.mu.Lock()
	if c.agentFeatures == nil {
		c.agentFeatures = make(map[string]acpAgentFeatures)
	}
	features.AuthMethods = slices.Clone(features.AuthMethods)
	c.agentFeatures[agentID] = features
	c.mu.Unlock()
}

func (c *controller) isClineAgent(agentID string) bool {
	c.mu.RLock()
	profile := c.profiles[agentID]
	c.mu.RUnlock()
	return isClineACPProfile(profile)
}

func (c *controller) additionalDirectoriesForAgent(agentID string, directories []string) []string {
	c.mu.RLock()
	supported := c.agentFeatures[agentID].SupportsAdditionalDirectories
	c.mu.RUnlock()
	if !supported {
		return nil
	}
	return slices.Clone(directories)
}

func (c *controller) agentSupportsSessionList(agentID string) bool {
	c.mu.RLock()
	supported := c.agentFeatures[agentID].SupportsSessionList
	c.mu.RUnlock()
	return supported
}

func (c *controller) agentSupportsSessionResume(agentID string) bool {
	c.mu.RLock()
	supported := c.agentFeatures[agentID].SupportsSessionResume
	c.mu.RUnlock()
	return supported
}

func (c *controller) setClinePermissionModeFromACP(agentID, sessionID, modeID string, options []acpConfigOption) {
	c.mu.Lock()
	session, ok := desktopSessionByID(c.state, sessionID, agentID)
	if !ok || session.AgentID != agentID || !isClineACPProfile(c.profiles[agentID]) {
		c.mu.Unlock()
		return
	}
	if current := desktopstateSessionPointer(&c.state, sessionID, agentID); current != nil {
		modeID = clineModeID(options, modeID)
		autoApprove := clineAutoApproveValue(options, current.Runtime.PermissionMode == "always-approve")
		current.Runtime.PermissionMode = clinePermissionMode(modeID, autoApprove)
		c.revision++
	}
	c.mu.Unlock()
	c.notify()
}

func acpConfigStringValue(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func clineAutoApproveValue(options []acpConfigOption, fallback bool) bool {
	for _, option := range options {
		if strings.TrimSpace(option.ID) != "auto_approve" {
			continue
		}
		switch value := option.CurrentValue.(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(strings.TrimSpace(value), "true")
		default:
			return fallback
		}
	}
	return fallback
}

func clineModeID(options []acpConfigOption, fallback string) string {
	for _, option := range options {
		if strings.TrimSpace(option.ID) == "mode" {
			if value := acpConfigStringValue(option.CurrentValue); value != "" {
				return value
			}
		}
	}
	if strings.TrimSpace(fallback) == "plan" {
		return "plan"
	}
	return "act"
}

func clinePermissionMode(modeID string, autoApprove bool) string {
	if strings.TrimSpace(modeID) == "plan" {
		return "plan"
	}
	if autoApprove {
		return "always-approve"
	}
	return "ask"
}

func setClinePermissionMode(ctx context.Context, client acpRPCClient, sessionID, permissionMode string) error {
	modeID := "act"
	autoApprove := false
	switch strings.TrimSpace(permissionMode) {
	case "ask":
	case "plan":
		modeID = "plan"
	case "always-approve":
		autoApprove = true
	default:
		return fmt.Errorf("unsupported Cline permission mode %q", permissionMode)
	}

	setAutoApprove := func(value bool) error {
		var result struct {
			ConfigOptions []acpConfigOption `json:"configOptions,omitempty"`
		}
		return client.Call(ctx, "session/set_config_option", map[string]any{
			"sessionId": sessionID,
			"configId":  "auto_approve",
			"value":     value,
		}, &result)
	}

	if !autoApprove {
		if err := setAutoApprove(false); err != nil {
			return fmt.Errorf("disable Cline auto-approval: %w", err)
		}
	}
	if err := client.Call(ctx, "session/set_mode", map[string]any{
		"sessionId": sessionID,
		"modeId":    modeID,
	}, &struct{}{}); err != nil {
		return fmt.Errorf("set Cline mode %s: %w", modeID, err)
	}
	if autoApprove {
		if err := setAutoApprove(true); err != nil {
			return fmt.Errorf("enable Cline auto-approval: %w", err)
		}
	}
	return nil
}

func isClineACPProfile(profile app.ACPAgentProfile) bool {
	for _, value := range []string{profile.ID, profile.DisplayName, profile.Command} {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), "cline") {
			return true
		}
	}
	return false
}

func preferredACPAuthMethod(profile app.ACPAgentProfile, methods []acpAuthMethod) (acpAuthMethod, bool) {
	isAgentMethod := func(method acpAuthMethod) bool {
		methodType := strings.TrimSpace(method.Type)
		return methodType == "" || methodType == "agent"
	}
	if isClineACPProfile(profile) {
		for _, method := range methods {
			if method.ID == "cline" && isAgentMethod(method) {
				return method, true
			}
		}
	}
	if len(methods) == 1 && strings.TrimSpace(methods[0].ID) != "" && isAgentMethod(methods[0]) {
		return methods[0], true
	}
	return acpAuthMethod{}, false
}

type acpRPCClient interface {
	Call(context.Context, string, any, any) error
}

func createACPSession(
	ctx context.Context,
	client acpRPCClient,
	profile app.ACPAgentProfile,
	authMethods []acpAuthMethod,
	params any,
	result any,
	onAuthenticating func(string),
) error {
	callNewSession := func() error {
		callCtx, cancel := context.WithTimeout(ctx, newSessionTimeout)
		defer cancel()
		return client.Call(callCtx, "session/new", params, result)
	}

	if err := callNewSession(); err != nil {
		var rpcErr *acpclient.RPCError
		if !errors.As(err, &rpcErr) || rpcErr.Code != acpErrorAuthRequired {
			return err
		}

		method, ok := preferredACPAuthMethod(profile, authMethods)
		if !ok {
			return err
		}
		if authErr := authenticateACPMethod(ctx, client, method, onAuthenticating); authErr != nil {
			return authErr
		}
		return callNewSession()
	}
	return nil
}

func promptACPWithReauthentication(
	ctx context.Context,
	client acpRPCClient,
	profile app.ACPAgentProfile,
	authMethods []acpAuthMethod,
	params any,
	result any,
	onAuthenticating func(string),
) error {
	promptErr := client.Call(ctx, "session/prompt", params, result)
	if promptErr == nil || !isClineACPProfile(profile) || !isACPReauthenticationRequired(promptErr) {
		return promptErr
	}
	method, ok := preferredACPAuthMethod(profile, authMethods)
	if !ok {
		return promptErr
	}
	if err := authenticateACPMethod(ctx, client, method, onAuthenticating); err != nil {
		return fmt.Errorf("prompt failed: %w; re-authentication failed: %v", promptErr, err)
	}
	return client.Call(ctx, "session/prompt", params, result)
}

func authenticateACPMethod(ctx context.Context, client acpRPCClient, method acpAuthMethod, onAuthenticating func(string)) error {
	if onAuthenticating != nil {
		onAuthenticating(method.Name)
	}
	authCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := client.Call(authCtx, "authenticate", map[string]any{"methodId": method.ID}, &struct{}{}); err != nil {
		name := strings.TrimSpace(method.Name)
		if name == "" {
			name = method.ID
		}
		return fmt.Errorf("authenticate with %s: %w", name, err)
	}
	return nil
}

func isACPReauthenticationRequired(err error) bool {
	var rpcErr *acpclient.RPCError
	if !errors.As(err, &rpcErr) {
		return false
	}
	var data struct {
		Details string `json:"details"`
		Message string `json:"message"`
	}
	if len(rpcErr.Data) > 0 {
		_ = json.Unmarshal(rpcErr.Data, &data)
	}
	detail := strings.ToLower(strings.TrimSpace(rpcErr.Message + " " + data.Details + " " + data.Message))
	if rpcErr.Code == acpErrorAuthRequired {
		return strings.Contains(detail, "authentication") || strings.Contains(detail, "credential") ||
			strings.Contains(detail, "re-auth") || strings.Contains(detail, "reauth")
	}
	if rpcErr.Code != -32603 {
		return false
	}
	return strings.Contains(detail, "re-auth") || strings.Contains(detail, "reauth") ||
		strings.Contains(detail, "authentication expired") || strings.Contains(detail, "token expired") ||
		strings.Contains(detail, "credential expired")
}
