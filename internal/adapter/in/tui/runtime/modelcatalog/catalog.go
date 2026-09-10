package modelcatalog

import (
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

type entry struct {
	models    []model.RemoteModel
	fetchedAt time.Time
}

type State struct {
	entries map[string]entry
}

func NormalizeProviderKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (s *State) Set(provider string, models []model.RemoteModel) {
	s.SetAt(provider, models, time.Now())
}

func (s *State) SetAt(provider string, models []model.RemoteModel, fetchedAt time.Time) {
	key := NormalizeProviderKey(provider)
	if key == "" {
		return
	}
	if s.entries == nil {
		s.entries = make(map[string]entry)
	}
	s.entries[key] = entry{models: append([]model.RemoteModel(nil), models...), fetchedAt: fetchedAt}
}

func (s *State) Len() int {
	if s == nil {
		return 0
	}
	return len(s.entries)
}

func (s *State) Has(provider string) bool {
	if s == nil || s.entries == nil {
		return false
	}
	_, ok := s.entries[NormalizeProviderKey(provider)]
	return ok
}

func (s *State) Delete(provider string) {
	if s == nil || s.entries == nil {
		return
	}
	delete(s.entries, NormalizeProviderKey(provider))
}

func (s *State) Models(provider string) []model.RemoteModel {
	if s == nil || s.entries == nil {
		return nil
	}
	catalog, ok := s.entries[NormalizeProviderKey(provider)]
	if !ok {
		return nil
	}
	return append([]model.RemoteModel(nil), catalog.models...)
}

func (s *State) FreshModels(provider string, now time.Time, ttl time.Duration) ([]model.RemoteModel, bool) {
	if s == nil || s.entries == nil {
		return nil, false
	}
	catalog, ok := s.entries[NormalizeProviderKey(provider)]
	if !ok || catalog.fetchedAt.IsZero() || ttl <= 0 || now.Sub(catalog.fetchedAt) >= ttl {
		return nil, false
	}
	return append([]model.RemoteModel(nil), catalog.models...), true
}
