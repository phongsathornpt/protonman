package runtime

import (
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

type modelCatalogState struct {
	modelcatalog.State
}

func normalizeProviderKey(name string) string                                { return modelcatalog.NormalizeProviderKey(name) }
func (s *modelCatalogState) set(provider string, models []model.RemoteModel) { s.Set(provider, models) }
func (s *modelCatalogState) setAt(provider string, models []model.RemoteModel, fetchedAt time.Time) {
	s.SetAt(provider, models, fetchedAt)
}
func (s *modelCatalogState) len() int                                   { return s.Len() }
func (s *modelCatalogState) has(provider string) bool                   { return s.Has(provider) }
func (s *modelCatalogState) delete(provider string)                     { s.Delete(provider) }
func (s *modelCatalogState) models(provider string) []model.RemoteModel { return s.Models(provider) }
func (s *modelCatalogState) freshModels(provider string, now time.Time, ttl time.Duration) ([]model.RemoteModel, bool) {
	return s.FreshModels(provider, now, ttl)
}

func (m *bubbleModel) modelIDKnown(provider, modelID string) bool {
	if m == nil {
		return false
	}
	for _, candidate := range m.modelCatalogs.models(provider) {
		if strings.EqualFold(strings.TrimSpace(candidate.ID), strings.TrimSpace(modelID)) {
			return true
		}
	}
	return false
}

func (m *bubbleModel) activeRemoteModel() (model.RemoteModel, bool) {
	if m == nil {
		return model.RemoteModel{}, false
	}
	for _, candidate := range m.modelCatalogs.models(m.activeProvider) {
		if strings.EqualFold(strings.TrimSpace(candidate.ID), strings.TrimSpace(m.activeModel)) {
			return candidate, true
		}
	}
	return model.RemoteModel{}, false
}
