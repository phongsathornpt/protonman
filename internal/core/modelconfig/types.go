// Package modelconfig owns provider-neutral model routing configuration.
package modelconfig

import sdk "github.com/phongsathornpt/protonman/proton-sdk"

// Provider describes one configured model provider connection.
type Provider struct {
	Name    string `toml:"name"`
	Type    string `toml:"type"`
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"api_key"`
}

// Selection identifies the active provider and model.
type Selection struct {
	Default  string `toml:"default"`
	Provider string `toml:"provider"`
}

// SubagentRoute contains a per-profile model/reasoning override.
type SubagentRoute struct {
	Provider        string              `toml:"provider"`
	Model           string              `toml:"model"`
	ReasoningEffort sdk.ReasoningEffort `toml:"reasoning_effort"`
}
