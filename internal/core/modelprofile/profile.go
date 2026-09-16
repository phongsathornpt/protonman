package modelprofile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
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

type ToolSchemaDialect string

const (
	ToolSchemaDefault      ToolSchemaDialect = ""
	ToolSchemaGeminiSubset ToolSchemaDialect = "gemini_openapi_subset"
)

// ThinkingMode describes the provider contract used to request model
// reasoning. It is transport metadata, not prompt policy.
type ThinkingMode string

const (
	ThinkingModeDefault     ThinkingMode = ""
	ThinkingModeAdaptive    ThinkingMode = "adaptive"
	ThinkingModeManual      ThinkingMode = "manual"
	ThinkingModeUnsupported ThinkingMode = "unsupported"
)

type MetadataSource string

const (
	MetadataSourceUnknown MetadataSource = ""
	MetadataSourceBuiltin MetadataSource = "builtin"
	MetadataSourceCatalog MetadataSource = "catalog"
)

type MetadataProvenance struct {
	Tools              MetadataSource
	Vision             MetadataSource
	ReasoningSupport   MetadataSource
	ReasoningLevels    MetadataSource
	ReasoningDefault   MetadataSource
	ToolChoiceRequired MetadataSource
	ThinkingMode       MetadataSource
	VisionPolicy       MetadataSource
	ForcedToolChoice   MetadataSource
	ContextWindow      MetadataSource
	MaxInputTokens     MetadataSource
	MaxOutputTokens    MetadataSource
	ToolSchemaDialect  MetadataSource
}

func (p MetadataProvenance) Summary() string {
	fields := []struct {
		name   string
		source MetadataSource
	}{
		{"tools", p.Tools}, {"vision", p.Vision}, {"reasoning_support", p.ReasoningSupport},
		{"reasoning_levels", p.ReasoningLevels}, {"reasoning_default", p.ReasoningDefault},
		{"tool_choice_required", p.ToolChoiceRequired}, {"context_window", p.ContextWindow},
		{"thinking_mode", p.ThinkingMode}, {"vision_policy", p.VisionPolicy},
		{"forced_tool_choice", p.ForcedToolChoice},
		{"max_input_tokens", p.MaxInputTokens}, {"max_output_tokens", p.MaxOutputTokens},
		{"tool_schema_dialect", p.ToolSchemaDialect},
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.source != MetadataSourceUnknown {
			parts = append(parts, field.name+"="+string(field.source))
		}
	}
	return strings.Join(parts, ",")
}

type MatchKind string

const (
	MatchNone     MatchKind = ""
	MatchFallback MatchKind = "fallback"
	MatchProvider MatchKind = "provider"
	MatchFamily   MatchKind = "family"
	MatchExact    MatchKind = "exact"
)

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
	Tools              Support
	Vision             Support
	Reasoning          Support
	ToolChoiceRequired Support
}

// Apply overlays model-level capability knowledge onto the capabilities
// published by the transport adapter. Unknown model fields deliberately keep
// the adapter value; explicit model metadata always wins.
func (c Capabilities) Apply(base domain.ModelCapabilities) domain.ModelCapabilities {
	if value, known := c.Tools.Bool(); known {
		base.Tools = value
	}
	if value, known := c.Vision.Bool(); known {
		base.Vision = value
	}
	return base
}

type Reasoning struct {
	Support Support
	Levels  []domain.ReasoningEffort
	Default domain.ReasoningEffort
}

type Sampling struct {
	Temperature Support
	TopP        Support
	TopK        Support
}

type CompatibilityPolicy struct {
	ToolSchemaDialect ToolSchemaDialect
	ThinkingMode      ThinkingMode
	ForcedToolChoice  Support
}

// CompactionPolicy controls when conversation history is compacted relative to
// the model's effective input budget. Zero-valued ratios inherit tier defaults.
type CompactionPolicy struct {
	SoftThresholdRatio       float64
	MediumThresholdRatio     float64
	AggressiveThresholdRatio float64
	EmergencyThresholdRatio  float64
	TargetRatio              float64
	MinRecentMessages        int
}

type Profile struct {
	Name            string
	Match           Matcher
	Capabilities    Capabilities
	Reasoning       Reasoning
	Sampling        Sampling
	ContextWindow   int
	MaxInputTokens  int
	MaxOutputTokens int
	Compatibility   CompatibilityPolicy
	Compaction      CompactionPolicy
	VisionPolicy    VisionPolicy
}

type CatalogReasoning struct {
	Supported *bool                    `json:"supported,omitempty"`
	Levels    []domain.ReasoningEffort `json:"levels,omitempty"`
	Default   domain.ReasoningEffort   `json:"default,omitempty"`
}

func NormalizeCatalogReasoning(value *CatalogReasoning) *CatalogReasoning {
	if value == nil {
		return nil
	}
	normalized := &CatalogReasoning{Supported: value.Supported}
	if value.Supported != nil && !*value.Supported {
		return normalized
	}
	seen := make(map[domain.ReasoningEffort]struct{}, len(value.Levels))
	for _, effort := range value.Levels {
		if !effort.Valid() || effort == domain.ReasoningDefault {
			continue
		}
		if _, ok := seen[effort]; ok {
			continue
		}
		seen[effort] = struct{}{}
		normalized.Levels = append(normalized.Levels, effort)
	}
	if value.Default.Valid() && value.Default != domain.ReasoningDefault {
		normalized.Default = value.Default
	}
	return normalized
}

type CatalogMetadata struct {
	Tools              *bool
	Vision             *bool
	ToolChoiceRequired *bool
	ContextWindow      int
	MaxInputTokens     int
	MaxOutputTokens    int
	Reasoning          *CatalogReasoning
	ToolSchemaDialect  ToolSchemaDialect
	ThinkingMode       ThinkingMode
	VisionPolicy       *VisionPolicy
}

type Resolved struct {
	ProfileName     string
	ProfileMatch    MatchKind
	CatalogOverride bool
	Provider        string
	ModelID         string
	Capabilities    Capabilities
	Reasoning       Reasoning
	Sampling        Sampling
	ContextWindow   int
	MaxInputTokens  int
	MaxOutputTokens int
	Compatibility   CompatibilityPolicy
	Compaction      CompactionPolicy
	VisionPolicy    VisionPolicy
	Provenance      MetadataProvenance
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
			resolved.ProfileMatch = matched.profile.Match.kind(provider, modelID)
		}
	}
	resolved.CatalogOverride = catalogHasMetadata(catalog)
	mergeCatalog(&resolved, catalog)
	return resolved
}

func (m Matcher) kind(provider, modelID string) MatchKind {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	modelLeaf := modelIDLeaf(modelID)
	for _, exact := range m.ExactIDs {
		exact = strings.ToLower(strings.TrimSpace(exact))
		if exact != "" && (exact == modelID || exact == modelLeaf) {
			return MatchExact
		}
	}
	for _, prefix := range m.Prefixes {
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix != "" && (strings.HasPrefix(modelID, prefix) || strings.HasPrefix(modelLeaf, prefix)) {
			return MatchFamily
		}
	}
	if strings.TrimSpace(m.Provider) != "" {
		return MatchProvider
	}
	return MatchFallback
}

func modelIDLeaf(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if slash := strings.LastIndexByte(modelID, '/'); slash >= 0 && slash+1 < len(modelID) {
		return modelID[slash+1:]
	}
	return modelID
}

func catalogHasMetadata(c CatalogMetadata) bool {
	validDialect := c.ToolSchemaDialect == ToolSchemaGeminiSubset
	validThinking := c.ThinkingMode == ThinkingModeAdaptive || c.ThinkingMode == ThinkingModeManual || c.ThinkingMode == ThinkingModeUnsupported
	validVisionPolicy := c.VisionPolicy != nil && validateVisionPolicy(*c.VisionPolicy) == nil
	return c.Tools != nil || c.Vision != nil || c.ToolChoiceRequired != nil || c.ContextWindow > 0 || c.MaxInputTokens > 0 || c.MaxOutputTokens > 0 || c.Reasoning != nil || validDialect || validThinking || validVisionPolicy
}

func (m Matcher) score(provider, modelID string) (int, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	modelLeaf := modelIDLeaf(modelID)
	if want := strings.ToLower(strings.TrimSpace(m.Provider)); want != "" && provider != want {
		return 0, false
	}
	score := 1
	if strings.TrimSpace(m.Provider) != "" {
		score += 10
	}
	for _, exact := range m.ExactIDs {
		exact = strings.ToLower(strings.TrimSpace(exact))
		if exact != "" && (exact == modelID || exact == modelLeaf) {
			return score + 10000, true
		}
	}
	bestPrefix := -1
	for _, prefix := range m.Prefixes {
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix != "" && (strings.HasPrefix(modelID, prefix) || strings.HasPrefix(modelLeaf, prefix)) && len(prefix) > bestPrefix {
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
