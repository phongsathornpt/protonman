package provider

import (
	"sort"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

type SelectionKind int

const (
	SelectionConfigured SelectionKind = iota
	SelectionPreset
	SelectionCustom
)

type SelectionEntry struct {
	Kind         SelectionKind
	Name         string
	DisplayName  string
	BaseURL      string
	APIKey       string
	Description  string
	PresetID     string
	IsConfigured bool
	IsActive     bool
	IsFree       bool
}

func BuildSelectionEntries(providers map[string]config.ProviderConfig, activeProvider string) []SelectionEntry {
	entries := make([]SelectionEntry, 0, len(providers)+len(model.SupportedPresets)+1)
	configured := make(map[string]bool, len(providers))
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cfg := providers[name]
		displayName := cfg.Name
		if displayName == "" {
			displayName = name
		}
		if preset := model.LookupPreset(name); preset != nil {
			displayName = preset.Name
		}
		entries = append(entries, SelectionEntry{
			Kind: SelectionConfigured, Name: name, DisplayName: displayName,
			BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, PresetID: name,
			IsConfigured: true, IsActive: strings.EqualFold(name, activeProvider),
			IsFree: model.IsProvider(model.DefaultOpenCodeName, name, cfg.BaseURL),
		})
		configured[strings.ToLower(name)] = true
	}
	for _, preset := range model.SupportedPresets {
		if configured[strings.ToLower(preset.ID)] {
			continue
		}
		entries = append(entries, SelectionEntry{
			Kind: SelectionPreset, Name: preset.ID, DisplayName: preset.Name,
			BaseURL: preset.BaseURL, Description: preset.Description, PresetID: preset.ID,
			IsActive: strings.EqualFold(preset.ID, activeProvider), IsFree: !preset.RequiresKey,
		})
	}
	entries = append(entries, SelectionEntry{
		Kind: SelectionCustom, Name: "custom", DisplayName: "+ Custom Gateway / Proxy",
		Description: "Any OpenAI-compatible or Anthropic Messages base URL",
	})
	return entries
}

func ActiveSelectionIndex(entries []SelectionEntry) int {
	for index, entry := range entries {
		if entry.IsActive {
			return index
		}
	}
	return 0
}
