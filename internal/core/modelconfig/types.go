// Package modelconfig owns provider-neutral model routing configuration.
package modelconfig

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

// ProviderName identifies a configured model provider. It is distinct from
// arbitrary strings so provider-routing APIs cannot accidentally receive a
// model ID or display label.
type ProviderName string

func (n ProviderName) String() string { return string(n) }

func (n ProviderName) Valid() bool { return n != "" }

func ParseProviderName(raw string) (ProviderName, error) {
	value := ProviderName(strings.TrimSpace(raw))
	if !value.Valid() {
		return "", fmt.Errorf("provider name cannot be empty")
	}
	return value, nil
}

// ModelID identifies a model within a provider route.
type ModelID string

func (id ModelID) String() string { return string(id) }

func (id ModelID) Valid() bool { return id != "" }

func ParseModelID(raw string) (ModelID, error) {
	value := ModelID(strings.TrimSpace(raw))
	if !value.Valid() {
		return "", fmt.Errorf("model ID cannot be empty")
	}
	return value, nil
}

// Provider describes one configured model provider connection.
type Provider struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// Selection identifies the active provider and model.
type Selection struct {
	Default  string `json:"default"`
	Provider string `json:"provider"`
}

// SubagentRoute contains a per-profile model/reasoning override.
type SubagentRoute struct {
	Provider        ProviderName           `json:"provider"`
	Model           ModelID                `json:"model"`
	ReasoningEffort domain.ReasoningEffort `json:"reasoning_effort"`
}
