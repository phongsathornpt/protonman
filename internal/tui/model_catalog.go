package tui

import (
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/model"
)

type providerModelCatalog struct {
	models    []model.RemoteModel
	fetchedAt time.Time
}

type modelCatalogState struct {
	entries map[string]providerModelCatalog
}

func normalizeProviderKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (s *modelCatalogState) set(provider string, models []model.RemoteModel) {
	s.setAt(provider, models, time.Now())
}

func (s *modelCatalogState) setAt(provider string, models []model.RemoteModel, fetchedAt time.Time) {
	key := normalizeProviderKey(provider)
	if key == "" {
		return
	}
	if s.entries == nil {
		s.entries = make(map[string]providerModelCatalog)
	}
	s.entries[key] = providerModelCatalog{
		models:    append([]model.RemoteModel(nil), models...),
		fetchedAt: fetchedAt,
	}
}

func (s *modelCatalogState) models(provider string) []model.RemoteModel {
	if s == nil || s.entries == nil {
		return nil
	}
	entry, ok := s.entries[normalizeProviderKey(provider)]
	if !ok {
		return nil
	}
	return append([]model.RemoteModel(nil), entry.models...)
}

func (s *modelCatalogState) freshModels(provider string, now time.Time, ttl time.Duration) ([]model.RemoteModel, bool) {
	if s == nil || s.entries == nil {
		return nil, false
	}
	entry, ok := s.entries[normalizeProviderKey(provider)]
	if !ok || entry.fetchedAt.IsZero() || ttl <= 0 || now.Sub(entry.fetchedAt) >= ttl {
		return nil, false
	}
	return append([]model.RemoteModel(nil), entry.models...), true
}

func (m *bubbleModel) modelIDKnown(provider, modelID string) bool {
	if m == nil {
		return false
	}
	models := m.modelCatalogs.models(provider)
	for _, candidate := range models {
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

func remoteModelSupportsVision(md model.RemoteModel) bool {
	for _, feature := range md.Features {
		if strings.EqualFold(strings.TrimSpace(feature), "vision") {
			return true
		}
	}
	return false
}

func remoteModelSupportsTools(md model.RemoteModel) bool {
	for _, feature := range md.Features {
		if strings.EqualFold(strings.TrimSpace(feature), "tools") {
			return true
		}
	}
	return false
}
