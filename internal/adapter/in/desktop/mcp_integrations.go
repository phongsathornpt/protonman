//go:build desktop

package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const mcpPreferencesKey = "mcp.integrations.v1"

func (a *application) initIntegrationControls() {
	a.integrationButton = widget.NewButton("MCP 0", func() {
		if a.integrationPanel.Visible() {
			a.integrationPanel.Hide()
		} else {
			a.integrationPanel.Show()
		}
	})
	a.integrationSummary = widget.NewLabel("No MCP integrations")
	a.integrationSummary.Wrapping = fyne.TextWrapWord
	a.integrationName = widget.NewEntry()
	a.integrationName.SetPlaceHolder("name")
	a.integrationCommand = widget.NewEntry()
	a.integrationCommand.SetPlaceHolder("command")
	a.integrationArgs = widget.NewEntry()
	a.integrationArgs.SetPlaceHolder(`args JSON, e.g. ["--stdio"]`)
	a.integrationEnv = widget.NewEntry()
	a.integrationEnv.SetPlaceHolder(`env keys JSON, e.g. ["TOKEN"]`)
	a.integrationSave = widget.NewButton("Save", a.saveIntegration)
	a.integrationRemove = widget.NewButton("Remove", a.removeIntegration)
	a.integrationReconnect = widget.NewButton("Reconnect ACP", a.reconnectWithIntegrations)
	a.integrationPanel = container.NewVBox(
		a.integrationSummary,
		a.integrationName,
		a.integrationCommand,
		a.integrationArgs,
		a.integrationEnv,
		container.NewHBox(a.integrationSave, a.integrationRemove, a.integrationReconnect),
	)
	a.integrationPanel.Hide()
	a.loadIntegrations()
}

func (a *application) loadIntegrations() {
	if a.preferences == nil {
		return
	}
	raw := strings.TrimSpace(a.preferences.String(mcpPreferencesKey))
	if raw == "" {
		a.renderIntegrations()
		return
	}
	var items []desktopstate.MCPIntegrationState
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		a.setStatus("MCP preferences ignored · " + err.Error())
		return
	}
	items, err := normalizeIntegrations(items)
	if err != nil {
		a.setStatus("MCP preferences ignored · " + err.Error())
		return
	}
	a.mu.Lock()
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventIntegrationsReplaced, Integrations: items})
	a.mu.Unlock()
	// Rewrite legacy KEY=value entries immediately so secrets are not retained in
	// Desktop preferences. Runtime values are resolved from the process environment.
	if err := a.persistIntegrations(items); err != nil {
		a.setStatus("MCP preferences migration failed · " + err.Error())
	}
	a.renderIntegrations()
}

func (a *application) saveIntegration() {
	name := strings.TrimSpace(a.integrationName.Text)
	command := strings.TrimSpace(a.integrationCommand.Text)
	if name == "" || command == "" {
		a.setStatus("MCP integration requires name and command")
		return
	}
	args, err := parseStringList(a.integrationArgs.Text)
	if err != nil {
		a.setStatus("Invalid MCP args · " + err.Error())
		return
	}
	env, err := parseEnvironmentKeys(a.integrationEnv.Text)
	if err != nil {
		a.setStatus("Invalid MCP env · " + err.Error())
		return
	}

	a.mu.Lock()
	items := cloneMCPIntegrations(a.state.Integrations)
	replaced := false
	for i := range items {
		if items[i].Name == name {
			items[i] = desktopstate.MCPIntegrationState{Name: name, Command: command, Args: args, Env: env}
			replaced = true
			break
		}
	}
	if !replaced {
		items = append(items, desktopstate.MCPIntegrationState{Name: name, Command: command, Args: args, Env: env})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventIntegrationsReplaced, Integrations: items})
	a.mu.Unlock()
	if err := a.persistIntegrations(items); err != nil {
		a.setStatus("Save MCP integration failed · " + err.Error())
		return
	}
	a.renderIntegrations()
	a.setStatus("MCP integration saved · env values resolve at runtime")
}

func (a *application) removeIntegration() {
	name := strings.TrimSpace(a.integrationName.Text)
	if name == "" {
		a.setStatus("Enter an MCP integration name to remove")
		return
	}
	a.mu.Lock()
	items := cloneMCPIntegrations(a.state.Integrations)
	out := items[:0]
	found := false
	for _, item := range items {
		if item.Name == name {
			found = true
			continue
		}
		out = append(out, item)
	}
	if found {
		a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventIntegrationsReplaced, Integrations: out})
	}
	a.mu.Unlock()
	if !found {
		a.setStatus("MCP integration not found · " + name)
		return
	}
	if err := a.persistIntegrations(out); err != nil {
		a.setStatus("Remove MCP integration failed · " + err.Error())
		return
	}
	a.renderIntegrations()
	a.setStatus("MCP integration removed · new sessions use updated config")
}

func (a *application) reconnectWithIntegrations() {
	a.mu.Lock()
	busy := false
	for _, session := range a.state.Sessions {
		if a.sessionBusyLocked(session.ID) {
			busy = true
			break
		}
	}
	a.mu.Unlock()
	if busy {
		a.setStatus("Cannot reconnect ACP while a session is active")
		return
	}
	client := a.currentClient()
	if client == nil {
		a.setStatus("MCP integrations saved · waiting for ACP")
		return
	}
	a.setStatus("Reconnecting ACP with MCP integrations…")
	go func() { _ = client.Close() }()
}

func (a *application) persistIntegrations(items []desktopstate.MCPIntegrationState) error {
	if a.preferences == nil {
		return fmt.Errorf("desktop preferences are unavailable")
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	a.preferences.SetString(mcpPreferencesKey, string(payload))
	return nil
}

func (a *application) renderIntegrations() {
	a.mu.Lock()
	items := cloneMCPIntegrations(a.state.Integrations)
	a.mu.Unlock()
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	summary := "No MCP integrations"
	if len(names) > 0 {
		summary = strings.Join(names, " · ")
	}
	a.integrationButton.SetText(fmt.Sprintf("MCP %d", len(items)))
	a.integrationSummary.SetText(summary)
}

func (a *application) mcpServersPayload() []map[string]any {
	a.mu.Lock()
	items := cloneMCPIntegrations(a.state.Integrations)
	a.mu.Unlock()
	servers := make([]map[string]any, 0, len(items))
	for _, item := range items {
		server := map[string]any{"name": item.Name, "command": item.Command}
		if len(item.Args) > 0 {
			server["args"] = append([]string(nil), item.Args...)
		}
		if env := resolveEnvironment(item.Env, os.LookupEnv); len(env) > 0 {
			server["env"] = env
		}
		servers = append(servers, server)
	}
	return servers
}

func parseStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("expected a JSON string array: %w", err)
	}
	out := values[:0]
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return append([]string(nil), out...), nil
}

func parseEnvironmentKeys(raw string) ([]string, error) {
	values, err := parseStringList(raw)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		if strings.Contains(value, "=") {
			return nil, fmt.Errorf("environment values are not stored; set %q in the Desktop process environment and enter only its variable name", strings.TrimSpace(strings.SplitN(value, "=", 2)[0]))
		}
	}
	return normalizeEnvironmentKeys(values)
}

func normalizeIntegrations(items []desktopstate.MCPIntegrationState) ([]desktopstate.MCPIntegrationState, error) {
	seen := make(map[string]struct{}, len(items))
	out := make([]desktopstate.MCPIntegrationState, 0, len(items))
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
		item.Args = trimStringList(item.Args)
		var err error
		item.Env, err = normalizeEnvironmentKeys(item.Env)
		if err != nil {
			return nil, fmt.Errorf("integration %q environment: %w", item.Name, err)
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func normalizeEnvironmentKeys(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := value
		if before, _, found := strings.Cut(value, "="); found {
			key = strings.TrimSpace(before)
		}
		if !validEnvironmentKey(key) {
			return nil, fmt.Errorf("invalid environment variable name %q", key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out, nil
}

func validEnvironmentKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if i == 0 {
			if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
				return false
			}
			continue
		}
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func resolveEnvironment(keys []string, lookup func(string) (string, bool)) []string {
	if lookup == nil {
		return nil
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := lookup(key); ok {
			out = append(out, key+"="+value)
		}
	}
	return out
}

func trimStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func cloneMCPIntegrations(items []desktopstate.MCPIntegrationState) []desktopstate.MCPIntegrationState {
	out := make([]desktopstate.MCPIntegrationState, len(items))
	for i, item := range items {
		out[i] = item
		out[i].Args = append([]string(nil), item.Args...)
		out[i].Env = append([]string(nil), item.Env...)
	}
	return out
}
