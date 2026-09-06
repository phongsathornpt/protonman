package tui

import (
	"strings"

	"github.com/projectTHORN/proton/internal/model"
)

type modelCatalogState struct {
	entries map[string][]model.RemoteModel
}

func normalizeProviderKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (s *modelCatalogState) set(provider string, models []model.RemoteModel) {
	key := normalizeProviderKey(provider)
	if key == "" {
		return
	}
	if s.entries == nil {
		s.entries = make(map[string][]model.RemoteModel)
	}
	s.entries[key] = append([]model.RemoteModel(nil), models...)
}

func (s *modelCatalogState) models(provider string) []model.RemoteModel {
	if s == nil || s.entries == nil {
		return nil
	}
	return append([]model.RemoteModel(nil), s.entries[normalizeProviderKey(provider)]...)
}
