package config

import "strings"

// ReconcileModelSelection keeps an effective provider/model pair usable when
// persisted configuration references a provider that no longer exists.
func ReconcileModelSelection(current ModelConfig, providers map[string]ProviderConfig) (ModelConfig, bool) {
	provider := strings.ToLower(strings.TrimSpace(current.Provider))
	if provider == "" {
		return current, false
	}
	if _, ok := providers[provider]; ok {
		changed := current.Provider != provider
		current.Provider = provider
		return current, changed
	}

	current.Default = ""
	current.Provider = ""
	return current, true
}
