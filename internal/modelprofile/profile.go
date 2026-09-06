package modelprofile

import (
	"fmt"
	"sort"
	"strings"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type Support uint8

const (
	SupportUnknown Support = iota
	SupportYes
	SupportNo
)

func (s Support) Bool() (bool, bool) {
	switch s {
	case SupportYes:
		return true, true
	case SupportNo:
		return false, true
	default:
		return false, false
	}
}

func supportFromPointer(value *bool) Support {
	if value == nil {
		return SupportUnknown
	}
	if *value {
		return SupportYes
	}
	return SupportNo
}

type Matcher struct {
	Provider string
	ExactIDs []string
	Prefixes []string
}

type Capabilities struct {
	Tools     Support
	Vision    Support
	Reasoning Support
}

type Reasoning struct {
	Support Support
	Levels  []sdk.ReasoningEffort
	Default sdk.ReasoningEffort
}

type Sampling struct {
	Temperature Support
	TopP        Support
	TopK        Support
}

type Profile struct {
	Name          string
	Match         Matcher
	Capabilities  Capabilities
	Reasoning     Reasoning
	Sampling      Sampling
	ContextWindow int
	PromptHints   []string
}

type CatalogReasoning struct {
	Supported *bool                 `json:"supported,omitempty"`
	Levels    []sdk.ReasoningEffort `json:"levels,omitempty"`
	Default   sdk.ReasoningEffort   `json:"default,omitempty"`
}

type CatalogMetadata struct {
	Tools         *bool
	Vision        *bool
	ContextWindow int
	Reasoning     *CatalogReasoning
}

type Resolved struct {
	ProfileName   string
	Provider      string
	ModelID       string
	Capabilities  Capabilities
	Reasoning     Reasoning
	Sampling      Sampling
	ContextWindow int
	PromptHints   []string
}

type Registry struct {
	profiles []Profile
}

func NewRegistry(profiles ...Profile) (*Registry, error) {
	seen := make(map[string]struct{}, len(profiles))
	copyProfiles := make([]Profile, 0, len(profiles))
	for _, profile := range profiles {
		profile.Name = strings.TrimSpace(profile.Name)
		if profile.Name == "" {
			return nil, fmt.Errorf("model profile name is required")
		}
		key := strings.ToLower(profile.Name)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate model profile %q", profile.Name)
		}
		seen[key] = struct{}{}
		if err := validateProfile(profile); err != nil {
			return nil, fmt.Errorf("model profile %q: %w", profile.Name, err)
		}
		copyProfiles = append(copyProfiles, cloneProfile(profile))
	}
	return &Registry{profiles: copyProfiles}, nil
}

func (r *Registry) Resolve(provider, modelID string, catalog CatalogMetadata) Resolved {
	resolved := Resolved{Provider: strings.TrimSpace(provider), ModelID: strings.TrimSpace(modelID)}
	if r != nil {
		type match struct {
			profile Profile
			score   int
		}
		matches := make([]match, 0, len(r.profiles))
		for _, profile := range r.profiles {
			if score, ok := profile.Match.score(provider, modelID); ok {
				matches = append(matches, match{profile: profile, score: score})
			}
		}
		sort.SliceStable(matches, func(i, j int) bool { return matches[i].score < matches[j].score })
		for _, matched := range matches {
			mergeProfile(&resolved, matched.profile)
		}
	}
	mergeCatalog(&resolved, catalog)
	return resolved
}

func (m Matcher) score(provider, modelID string) (int, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	if want := strings.ToLower(strings.TrimSpace(m.Provider)); want != "" && provider != want {
		return 0, false
	}
	score := 1
	if strings.TrimSpace(m.Provider) != "" {
		score += 10
	}
	for _, exact := range m.ExactIDs {
		if strings.EqualFold(strings.TrimSpace(exact), modelID) {
			return score + 10000, true
		}
	}
	bestPrefix := -1
	for _, prefix := range m.Prefixes {
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix != "" && strings.HasPrefix(modelID, prefix) && len(prefix) > bestPrefix {
			bestPrefix = len(prefix)
		}
	}
	if bestPrefix >= 0 {
		return score + 100 + bestPrefix, true
	}
	if len(m.ExactIDs) == 0 && len(m.Prefixes) == 0 {
		return score, true
	}
	return 0, false
}
