package provider

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

type EditorDraft struct {
	PresetID       string
	Name           string
	Endpoint       string
	KeyPlaceholder string
	ProviderType   string
	RequiresAPIKey bool
	FilterFreeOnly bool
	FocusField     Field
}

func NewEditorDraft(preset string) EditorDraft {
	preset = strings.ToLower(strings.TrimSpace(preset))
	draft := EditorDraft{
		KeyPlaceholder: "API key (optional)…",
		ProviderType:   string(model.ProviderProtocolOpenAI),
		FocusField:     FieldName,
	}
	if p := model.LookupPreset(preset); p != nil {
		draft.PresetID = p.ID
		draft.Name = p.ID
		draft.Endpoint = p.BaseURL
		draft.KeyPlaceholder = KeyPlaceholder(*p)
		draft.ProviderType = string(p.Protocol)
		draft.RequiresAPIKey = p.RequiresKey
		draft.FilterFreeOnly = p.ID == model.DefaultOpenCodeName
	} else if preset == "opencode-free" || preset == "free" {
		draft.PresetID = model.DefaultOpenCodeName
		draft.Name = model.DefaultOpenCodeName
		draft.Endpoint = model.DefaultOpenCodeEndpoint
		draft.FilterFreeOnly = true
	} else if preset != "" {
		draft.Name = preset
	}
	if draft.Name != "" && draft.Endpoint != "" {
		draft.FocusField = FieldAPIKey
	}
	return draft
}
