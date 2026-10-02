//go:build desktop || desktop_gio

package shell

import (
	"gioui.org/layout"

	settingscomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/settings"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

var defaultPresetCatalog = []desktopstate.ProviderState{
	{ID: "opencode", Name: "OpenCode (Free)", Protocol: "openai", RequiresKey: false, IsFree: true},
	{ID: "opencode-zen", Name: "OpenCode Zen", Protocol: "openai", RequiresKey: true},
	{ID: "opencode-go", Name: "OpenCode Go", Protocol: "openai", RequiresKey: true},
	{ID: "protonman", Name: "Protonman", Protocol: "openai", RequiresKey: true},
	{ID: "ollama", Name: "Ollama (Local)", Protocol: "openai", RequiresKey: false, IsFree: true},
	{ID: "openai", Name: "OpenAI Official", Protocol: "openai", RequiresKey: true},
	{ID: "anthropic", Name: "Anthropic", Protocol: "anthropic", RequiresKey: true},
}

func (s *Shell) openProviderForm(id, name, endpoint, protocol, defaultModel string) {
	s.settingsComponent.OpenProviderForm(id, name, endpoint, protocol, defaultModel)
}

func (s *Shell) layoutProvidersPanel(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	providers := snapshot.Providers
	if len(providers) == 0 {
		providers = defaultPresetCatalog
	}
	items := make([]settingscomponent.ProviderItem, 0, len(providers))
	for _, provider := range providers {
		items = append(items, settingscomponent.ProviderItem{
			ID: provider.ID, Name: provider.Name, Protocol: provider.Protocol,
			BaseURL: provider.BaseURL, RequiresKey: provider.RequiresKey,
			HasKey: provider.HasKey, IsActive: provider.IsActive,
			IsFree: provider.IsFree, DefaultModel: provider.DefaultModel,
		})
	}
	return s.settingsComponent.LayoutProviders(gtx, settingscomponent.ProvidersInput{
		Providers: items, Updating: snapshot.ProviderUpdating, Chrome: s.settingsChrome(),
		OnAdd: func() { s.openProviderForm("", "", "https://api.example.com/v1", "openai", "") },
		OnSelect: func(item settingscomponent.ProviderItem) {
			s.openProviderForm(item.ID, item.Name, item.BaseURL, item.Protocol, item.DefaultModel)
		},
		Form: func(gtx layout.Context) layout.Dimensions {
			return s.layoutProviderForm(gtx, snapshot, !snapshot.ProviderUpdating)
		},
	})
}

func (s *Shell) layoutProviderForm(gtx layout.Context, snapshot controller.Snapshot, enabled bool) layout.Dimensions {
	return s.settingsComponent.LayoutProviderForm(gtx, settingscomponent.ProviderFormInput{
		SelectedID:            s.settingsComponent.ProviderWidgets().FormSelectedID,
		ProviderModels:        snapshot.ProviderModels,
		ProviderModelsLoading: snapshot.ProviderModelsLoading,
		Updating:              !enabled,
		Chrome:                s.settingsChrome(),
		OnSave: func(params settingscomponent.ProviderSaveParams, cb func(error)) {
			if s.bind.SaveProvider != nil {
				s.bind.SaveProvider(controller.ProvidersSaveParams{
					ProviderName: params.ProviderName,
					ProviderType: params.ProviderType,
					PreviousName: params.PreviousName,
					BaseURL:      params.BaseURL,
					APIKey:       params.APIKey,
					DefaultModel: params.DefaultModel,
					Activate:     params.Activate,
				}, cb)
			}
		},
		OnDelete: func(name string, cb func(error)) {
			if s.bind.DeleteProvider != nil {
				s.bind.DeleteProvider(name, cb)
			}
		},
		OnFetchModels: func(id, endpoint, key, proto string, cb func([]string, error)) {
			if s.bind.FetchProviderModels != nil {
				s.bind.FetchProviderModels(id, endpoint, key, proto, cb)
			}
		},
	})
}
