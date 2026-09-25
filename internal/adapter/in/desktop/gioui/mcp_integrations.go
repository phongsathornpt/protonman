//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func (c *controller) loadMCPIntegrations() {
	if !c.mcpIntegrations.Available() {
		return
	}
	if c.statuses == nil {
		c.statuses = make(map[string]string)
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := c.mcpIntegrations.Load(ctx)
	if err != nil {
		c.mcpError = compactError(err)
		c.statuses[c.activeAgentID] = "MCP settings unavailable · " + c.mcpError
		c.revision++
		return
	}
	c.mcpError = ""
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:         desktopstate.EventIntegrationsReplaced,
		Integrations: desktopMCPIntegrations(items),
	})
}

func (c *controller) saveMCPIntegration(name, command, argsJSON, envJSON string) {
	name = strings.TrimSpace(name)
	command = strings.TrimSpace(command)
	if name == "" || command == "" {
		c.setMCPStatus("MCP integration requires name and command")
		return
	}
	args, err := parseMCPStringList(argsJSON)
	if err != nil {
		c.setMCPStatus("Invalid MCP args · " + compactError(err))
		return
	}
	env, err := parseMCPEnvironmentKeys(envJSON)
	if err != nil {
		c.setMCPStatus("Invalid MCP env · " + compactError(err))
		return
	}

	next, err := upsertMCPIntegration(c.snapshot().State.Integrations, desktopstate.MCPIntegrationState{
		Name: name, Command: command, Args: args, Env: env,
	})
	if err != nil {
		c.setMCPStatus("Invalid MCP integration · " + compactError(err))
		return
	}
	c.persistMCPIntegrations(next, "MCP integration saved · env values resolve at runtime")
}

func (c *controller) removeMCPIntegration(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		c.setMCPStatus("Enter an MCP integration name to remove")
		return
	}
	items := cloneDesktopMCPIntegrations(c.snapshot().State.Integrations)
	next := make([]desktopstate.MCPIntegrationState, 0, len(items))
	found := false
	for _, item := range items {
		if item.Name == name {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		c.setMCPStatus("MCP integration not found · " + name)
		return
	}
	c.persistMCPIntegrations(next, "MCP integration removed · reconnect to apply")
}

func (c *controller) persistMCPIntegrations(items []desktopstate.MCPIntegrationState, successStatus string) {
	c.mu.Lock()
	if c.statuses == nil {
		c.statuses = make(map[string]string)
	}
	if c.mcpMutation {
		c.mu.Unlock()
		c.setMCPStatus("MCP settings update already in progress")
		return
	}
	if !c.mcpIntegrations.Available() {
		c.mu.Unlock()
		c.setMCPStatus("MCP settings repository is unavailable")
		return
	}
	c.mcpMutation = true
	c.statuses[c.activeAgentID] = "Saving MCP settings…"
	c.revision++
	c.mu.Unlock()
	c.notify()

	go func() {
		ctx := c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		err := c.mcpIntegrations.Save(ctx, appMCPIntegrations(items))
		c.mu.Lock()
		c.mcpMutation = false
		if err != nil {
			c.mcpError = compactError(err)
			c.statuses[c.activeAgentID] = "MCP settings save failed · " + c.mcpError
		} else {
			c.mcpError = ""
			desktopstate.Apply(&c.state, desktopstate.Event{
				Kind:         desktopstate.EventIntegrationsReplaced,
				Integrations: cloneDesktopMCPIntegrations(items),
			})
			c.statuses[c.activeAgentID] = successStatus
		}
		c.revision++
		c.mu.Unlock()
		c.notify()
	}()
}

func (c *controller) reconnectMCP() {
	c.mu.Lock()
	if c.mcpReconnect {
		c.mu.Unlock()
		return
	}
	if c.mcpMutation {
		c.mu.Unlock()
		c.setMCPStatus("Wait for MCP settings to finish saving")
		return
	}
	for _, session := range c.state.Sessions {
		if sessionBusy(session.Status) {
			c.mu.Unlock()
			c.setMCPStatus("Cannot reconnect ACP while a session is active")
			return
		}
	}
	clients := make([]*acpclient.Client, 0, len(c.clients))
	for _, client := range c.clients {
		clients = append(clients, client)
	}
	if len(clients) == 0 {
		c.mu.Unlock()
		c.setMCPStatus("MCP integrations saved · waiting for ACP")
		return
	}
	c.mcpReconnect = true
	c.statuses[c.activeAgentID] = "Reconnecting ACP with MCP integrations…"
	c.revision++
	c.mu.Unlock()
	c.notify()

	go func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}()
}

func (c *controller) setMCPStatus(status string) {
	c.mu.Lock()
	if c.statuses == nil {
		c.statuses = make(map[string]string)
	}
	c.statuses[c.activeAgentID] = status
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) mcpSessionParams(sessionID, workspace string, additionalDirectories []string) map[string]any {
	return c.mcpSessionParamsWithServers(sessionID, workspace, additionalDirectories, c.mcpServersPayload())
}

func (c *controller) mcpSessionParamsWithServers(sessionID, workspace string, additionalDirectories []string, servers []map[string]any) map[string]any {
	params := map[string]any{
		"sessionId": sessionID,
		"cwd":       workspace,
	}
	if len(additionalDirectories) > 0 {
		params["additionalDirectories"] = append([]string(nil), additionalDirectories...)
	}
	if len(servers) > 0 {
		params["mcpServers"] = servers
	}
	return params
}

func (c *controller) mcpNewSessionParams(workspace string, additionalDirectories []string) map[string]any {
	params := map[string]any{"cwd": workspace}
	// docs/desktop.md promises the primary folder as cwd and the remaining
	// project folders as additionalDirectories for new sessions too. Dropping
	// them here silently narrows what the session is authorized to touch.
	if len(additionalDirectories) > 0 {
		params["additionalDirectories"] = append([]string(nil), additionalDirectories...)
	}
	if servers := c.mcpServersPayload(); len(servers) > 0 {
		params["mcpServers"] = servers
	}
	return params
}

func (c *controller) mcpServersPayload() []map[string]any {
	c.mu.RLock()
	items := cloneDesktopMCPIntegrations(c.state.Integrations)
	c.mu.RUnlock()
	return buildMCPServersPayload(items, os.LookupEnv)
}

func buildMCPServersPayload(items []desktopstate.MCPIntegrationState, lookup func(string) (string, bool)) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	servers := make([]map[string]any, 0, len(items))
	for _, item := range items {
		server := map[string]any{
			"name":    item.Name,
			"command": item.Command,
		}
		if len(item.Args) > 0 {
			server["args"] = append([]string(nil), item.Args...)
		}
		if env := resolveMCPEnvironment(item.Env, lookup); len(env) > 0 {
			server["env"] = env
		}
		servers = append(servers, server)
	}
	return servers
}

func parseMCPStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("expected a JSON string array: %w", err)
	}
	return trimMCPStringList(values), nil
}

func parseMCPEnvironmentKeys(raw string) ([]string, error) {
	values, err := parseMCPStringList(raw)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		if strings.Contains(value, "=") {
			key := strings.TrimSpace(strings.SplitN(value, "=", 2)[0])
			return nil, fmt.Errorf("environment values are not stored; set %q in the Protonman process environment and enter only its variable name", key)
		}
	}
	return normalizeMCPEnvironmentKeys(values)
}

func normalizeMCPIntegrations(items []desktopstate.MCPIntegrationState) ([]desktopstate.MCPIntegrationState, error) {
	seen := make(map[string]struct{}, len(items))
	normalized := make([]desktopstate.MCPIntegrationState, 0, len(items))
	for _, item := range items {
		item.Name = strings.TrimSpace(item.Name)
		item.Command = strings.TrimSpace(item.Command)
		if item.Name == "" || item.Command == "" {
			return nil, fmt.Errorf("integration name and command must be non-empty")
		}
		if _, exists := seen[item.Name]; exists {
			return nil, fmt.Errorf("duplicate integration %q", item.Name)
		}
		seen[item.Name] = struct{}{}
		item.Args = trimMCPStringList(item.Args)
		var err error
		item.Env, err = normalizeMCPEnvironmentKeys(item.Env)
		if err != nil {
			return nil, fmt.Errorf("integration %q environment: %w", item.Name, err)
		}
		normalized = append(normalized, cloneDesktopMCPIntegration(item))
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Name < normalized[j].Name })
	return normalized, nil
}

func upsertMCPIntegration(items []desktopstate.MCPIntegrationState, replacement desktopstate.MCPIntegrationState) ([]desktopstate.MCPIntegrationState, error) {
	next := cloneDesktopMCPIntegrations(items)
	replaced := false
	for index := range next {
		if next[index].Name == replacement.Name {
			next[index] = replacement
			replaced = true
			break
		}
	}
	if !replaced {
		next = append(next, replacement)
	}
	return normalizeMCPIntegrations(next)
}

func normalizeMCPEnvironmentKeys(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := value
		if before, _, found := strings.Cut(value, "="); found {
			key = strings.TrimSpace(before)
		}
		if !validMCPEnvironmentKey(key) {
			return nil, fmt.Errorf("invalid environment variable name %q", key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized, nil
}

func validMCPEnvironmentKey(key string) bool {
	if key == "" {
		return false
	}
	for index, character := range key {
		if index == 0 {
			if !(character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z') {
				return false
			}
			continue
		}
		if !(character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func resolveMCPEnvironment(keys []string, lookup func(string) (string, bool)) []string {
	if lookup == nil {
		return nil
	}
	resolved := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := lookup(key); ok {
			resolved = append(resolved, key+"="+value)
		}
	}
	return resolved
}

func trimMCPStringList(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func cloneDesktopMCPIntegration(item desktopstate.MCPIntegrationState) desktopstate.MCPIntegrationState {
	item.Args = append([]string(nil), item.Args...)
	item.Env = append([]string(nil), item.Env...)
	return item
}

func cloneDesktopMCPIntegrations(items []desktopstate.MCPIntegrationState) []desktopstate.MCPIntegrationState {
	cloned := make([]desktopstate.MCPIntegrationState, len(items))
	for index, item := range items {
		cloned[index] = cloneDesktopMCPIntegration(item)
	}
	return cloned
}

func desktopMCPIntegrations(items []app.MCPIntegration) []desktopstate.MCPIntegrationState {
	converted := make([]desktopstate.MCPIntegrationState, len(items))
	for index, item := range items {
		converted[index] = desktopstate.MCPIntegrationState{
			Name:    item.Name,
			Command: item.Command,
			Args:    append([]string(nil), item.Args...),
			Env:     append([]string(nil), item.Env...),
		}
	}
	return converted
}

func appMCPIntegrations(items []desktopstate.MCPIntegrationState) []app.MCPIntegration {
	converted := make([]app.MCPIntegration, len(items))
	for index, item := range items {
		converted[index] = app.MCPIntegration{
			Name:    item.Name,
			Command: item.Command,
			Args:    append([]string(nil), item.Args...),
			Env:     append([]string(nil), item.Env...),
		}
	}
	return converted
}
