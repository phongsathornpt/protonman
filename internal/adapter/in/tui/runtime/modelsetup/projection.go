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
	parts := make([]string, 0, 9)
	if id := strings.TrimSpace(item.Model.ID); id != "" {
		parts = append(parts, id)
	}
	if name := strings.TrimSpace(item.Model.Name); name != "" && !strings.EqualFold(name, item.Model.ID) {
		parts = append(parts, name)
	}
	if display := strings.TrimSpace(modelpicker.DisplayName(item.Model)); display != "" && !strings.EqualFold(display, item.Model.ID) && !strings.EqualFold(display, item.Model.Name) {
		parts = append(parts, display)
	}
	if prov := strings.TrimSpace(item.ProviderName); prov != "" {
		parts = append(parts, prov)
	}
	if vendor := strings.TrimSpace(item.Model.Provider); vendor != "" && !strings.EqualFold(vendor, item.ProviderName) {
		parts = append(parts, vendor)
	}
	if model.IsFreeModel(item.Model.ID) {
		parts = append(parts, "free")
	}
	if len(item.Model.Features) > 0 {
		parts = append(parts, strings.Join(item.Model.Features, " "))
	}
	resolved := model.ResolveRemoteMetadata(item.ProviderName, item.Model)
	if limits := modelpicker.FormatTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens); limits != "" {
		parts = append(parts, limits)
	}
	if reasoning := reasoningpolicy.Summary(item.ProviderName, item.Model, true); reasoning != "" {
		parts = append(parts, "reasoning thinking "+reasoning)
	}
	return strings.Join(parts, " ")
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
