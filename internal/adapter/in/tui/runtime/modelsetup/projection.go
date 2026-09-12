package modelsetup

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelpicker"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

type Projection struct {
	Model        model.RemoteModel
	ProviderName string
	Current      bool
}

func Project(models []model.RemoteModel, providerName, activeProvider, activeModel string) []Projection {
	currentProvider := strings.EqualFold(providerName, activeProvider)
	out := make([]Projection, 0, len(models))
	for _, md := range models {
		out = append(out, Projection{Model: md, ProviderName: providerName, Current: currentProvider && strings.EqualFold(md.ID, activeModel)})
	}
	return out
}

func FilterValue(item Projection) string {
	return strings.Join([]string{item.Model.ID, item.Model.Name, item.Model.Provider, strings.Join(item.Model.Features, " ")}, " ")
}
func Title(item Projection) string {
	return modelpicker.DisplayName(item.Model)
}

func Metadata(item Projection) []string {
	metadata := make([]string, 0, 2)
	if model.IsFreeModel(item.Model.ID) {
		metadata = append(metadata, "FREE")
	}
	if item.Current {
		metadata = append(metadata, "(current)")
	}
	return metadata
}

func Description(item Projection) string {
	parts := make([]string, 0, 4)
	if name := strings.TrimSpace(item.Model.Name); name != "" && !strings.EqualFold(name, item.Model.ID) {
		parts = append(parts, item.Model.ID)
	}
	resolved := model.ResolveRemoteMetadata(item.ProviderName, item.Model)
	if limits := modelpicker.FormatTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens); limits != "" {
		parts = append(parts, limits)
	}
	if len(resolved.Features) > 0 {
		parts = append(parts, strings.Join(resolved.Features, ", "))
	}
	if reasoning := reasoningpolicy.Summary(item.ProviderName, item.Model, true); reasoning != "" {
		parts = append(parts, reasoning)
	}
	return strings.Join(parts, " · ")
}
