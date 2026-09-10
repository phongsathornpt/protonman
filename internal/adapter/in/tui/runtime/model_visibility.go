package runtime

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func visibleModelsForAccess(providerName, baseURL, apiKey string, models []model.RemoteModel) []model.RemoteModel {
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
