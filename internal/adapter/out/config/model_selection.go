package config

import (
	"sort"
	"strings"
)

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

	keys := make([]string, 0, len(providers))
	for key := range providers {
		keys = append(keys, strings.ToLower(strings.TrimSpace(key)))
	}
	sort.Strings(keys)
	current.Default = ""
	current.Provider = ""
	if len(keys) > 0 {
		current.Provider = keys[0]
	}
	return current, true
}
