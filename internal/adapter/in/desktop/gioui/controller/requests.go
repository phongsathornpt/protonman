//go:build desktop || desktop_gio

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
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

func (c *controller) additionalDirectories(directories []string) []string {
	c.mu.RLock()
	supported := c.features.SupportsAdditionalDirectories
	c.mu.RUnlock()
	if !supported {
		return nil
	}
	return slices.Clone(directories)
}

func (c *controller) supportsSessionList() bool {
	c.mu.RLock()
	supported := c.features.SupportsSessionList
	c.mu.RUnlock()
	return supported
}

func (c *controller) supportsSessionResume() bool {
	c.mu.RLock()
	supported := c.features.SupportsSessionResume
	c.mu.RUnlock()
	return supported
}

func acpConfigStringValue(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func preferredACPAuthMethod(_ ACPAgentProfile, methods []acpAuthMethod) (acpAuthMethod, bool) {
	isAgentMethod := func(method acpAuthMethod) bool {
		methodType := strings.TrimSpace(method.Type)
		return methodType == "" || methodType == "agent"
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
	profile ACPAgentProfile,
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
	profile ACPAgentProfile,
	authMethods []acpAuthMethod,
	params any,
	result any,
	onAuthenticating func(string),
) error {
	err := client.Call(ctx, "session/prompt", params, result)
	if err == nil || !isACPReauthenticationRequired(err) {
		return err
	}
	firstErr := err
	method, ok := preferredACPAuthMethod(profile, authMethods)
	if !ok {
		return firstErr
	}
	if authErr := authenticateACPMethod(ctx, client, method, onAuthenticating); authErr != nil {
		return authErr
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
