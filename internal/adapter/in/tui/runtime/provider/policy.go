package provider

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func KeyPlaceholder(p model.SupportedProviderPreset) string {
	if strings.TrimSpace(p.KeyPlaceholder) != "" {
		return p.KeyPlaceholder
	}
	if !p.RequiresKey {
		return "API key (optional)…"
	}
	return "API key…"
}

func ProtocolLabel(providerType, presetID string) string {
	label := strings.ToLower(strings.TrimSpace(providerType))
	if label == "" {
		label = string(model.ProviderProtocolOpenAI)
	}
	if strings.TrimSpace(presetID) == "" {
		return label + " · ctrl+r to switch"
	}
	return label
}
func ToggleProtocol(providerType, presetID string) string {
	if strings.TrimSpace(presetID) != "" {
		return providerType
	}
	switch model.ProviderProtocol(strings.ToLower(strings.TrimSpace(providerType))) {
	case model.ProviderProtocolAnthropic:
		return string(model.ProviderProtocolOpenAI)
	default:
		return string(model.ProviderProtocolAnthropic)
	}
}
