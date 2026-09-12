package modelcatalog

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

// VisibleForAccess filters a provider catalog to models available to the current access mode.
func VisibleForAccess(providerName, baseURL, apiKey string, models []model.RemoteModel) []model.RemoteModel {
	if !model.IsProvider(model.DefaultOpenCodeName, providerName, baseURL) || strings.TrimSpace(apiKey) != "" {
		return models
	}
	visible := make([]model.RemoteModel, 0, len(models))
	for _, candidate := range models {
		if model.IsFreeModel(candidate.ID) {
			visible = append(visible, candidate)
		}
	}
	return visible
}
