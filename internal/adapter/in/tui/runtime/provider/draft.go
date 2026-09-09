package provider

import (
	"net/url"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

type Field int

const (
	FieldName Field = iota
	FieldEndpoint
	FieldAPIKey
	FieldCount
)

type Validation struct {
	Name       string
	Endpoint   string
	APIKey     string
	Errors     [FieldCount]string
	FirstError Field
	Valid      bool
}

func Validate(name, endpoint, apiKey string, requiresAPIKey bool) Validation {
	out := Validation{Name: strings.TrimSpace(name), Endpoint: strings.TrimSpace(endpoint), APIKey: strings.TrimSpace(apiKey), FirstError: FieldCount, Valid: true}
	if out.Name == "" {
		out.Errors[FieldName] = "required"
		out.FirstError = FieldName
		out.Valid = false
	}
	if out.Endpoint == "" {
		out.Errors[FieldEndpoint] = "required"
		if out.FirstError == FieldCount {
			out.FirstError = FieldEndpoint
		}
		out.Valid = false
	} else if !ValidEndpoint(out.Endpoint) {
		out.Errors[FieldEndpoint] = "use an HTTP(S) URL"
		if out.FirstError == FieldCount {
			out.FirstError = FieldEndpoint
		}
		out.Valid = false
	}
	if requiresAPIKey && out.APIKey == "" {
		out.Errors[FieldAPIKey] = "required for this provider"
		if out.FirstError == FieldCount {
			out.FirstError = FieldAPIKey
		}
		out.Valid = false
	}
	if out.Valid {
		out.Endpoint = strings.TrimRight(out.Endpoint, "/")
	}
	return out
}

func ValidEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

func HasNameConflict(providers map[string]config.ProviderConfig, name, original string) bool {
	name, original = strings.TrimSpace(name), strings.TrimSpace(original)
	for providerKey, cfg := range providers {
		existing := strings.TrimSpace(cfg.Name)
		if existing == "" {
			existing = providerKey
		}
		if strings.EqualFold(existing, name) && !strings.EqualFold(existing, original) {
			return true
		}
	}
	return false
}

func CurrentModels(models []model.RemoteModel, freeOnly bool) []model.RemoteModel {
	if !freeOnly {
		return models
	}
	filtered := make([]model.RemoteModel, 0, len(models))
	for _, candidate := range models {
		if model.IsFreeModel(candidate.ID) {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		return models
	}
	return filtered
}

func SortFetchedModels(models []model.RemoteModel, openCode bool) ([]model.RemoteModel, bool) {
	if !openCode {
		return models, false
	}
	freeList := make([]model.RemoteModel, 0, len(models))
	paidList := make([]model.RemoteModel, 0, len(models))
	for _, candidate := range models {
		if model.IsFreeModel(candidate.ID) {
			freeList = append(freeList, candidate)
		} else {
			paidList = append(paidList, candidate)
		}
	}
	return append(freeList, paidList...), len(freeList) > 0
}
