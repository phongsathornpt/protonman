// Package modelcatalogpolicy owns provider-neutral knowledge about curated
// model identifiers: which models a provider offers as a fallback, how they are
// labelled, and whether an identifier denotes a free-tier model.
//
// This is curated-catalog policy, distinct from internal/core/modelcatalog,
// which owns metadata discovered from a live provider catalog. Presentation
// packages ask these questions to decide grouping and badges; they must not
// carry their own copy of provider or model identifiers, because those copies
// drift.
package modelcatalogpolicy

import "strings"

// curatedFallbackModels lists the models a provider is assumed to expose when
// discovery has not answered yet.
//
// This is a presentation fallback for an empty catalog, not an authoritative
// list: a discovered model always wins. Entries are model identifiers only, so
// the caller can label them.
var curatedFallbackModels = map[string][]string{
	"protonman":    {"claude-3-7-sonnet-20250219", "claude-3-5-sonnet-20241022", "gpt-4o", "o3-mini", "deepseek-reasoner"},
	"openai":       {"gpt-4o", "gpt-4o-mini", "o3-mini", "o1"},
	"anthropic":    {"claude-3-7-sonnet-20250219", "claude-3-5-sonnet-20241022", "claude-3-5-haiku-20241022"},
	"opencode":     {"nemotron-3-super-free", "big-pickle", "mimo-v2.5-free"},
	"opencode-zen": {"claude-3-7-sonnet-20250219", "gpt-4o", "o3-mini"},
	"opencode-go":  {"claude-3-7-sonnet-20250219", "gpt-4o"},
	"ollama":       {"llama3.3", "qwen2.5-coder", "deepseek-r1"},
}

// genericFallbackModels is returned for a provider with no curated entry so the
// model picker still offers something rather than rendering an empty list.
var genericFallbackModels = []string{"claude-3-7-sonnet-20250219", "gpt-4o"}

// FallbackModelsForProvider returns the curated model identifiers assumed for a
// provider when no catalog is available. The returned slice is a copy, so a
// caller cannot mutate the curated table.
func FallbackModelsForProvider(provider string) []string {
	key := strings.ToLower(strings.TrimSpace(provider))
	if models, ok := curatedFallbackModels[key]; ok {
		return append([]string(nil), models...)
	}
	return append([]string(nil), genericFallbackModels...)
}

// curatedLabels maps a curated model identifier to its human label. Both the ACP
// session-config picker and the desktop quick-pick list read labels from here so
// the same model is never labelled two ways.
var curatedLabels = map[string]string{
	"claude-3-7-sonnet-20250219": "Claude 3.7 Sonnet",
	"claude-3-5-sonnet-20241022": "Claude 3.5 Sonnet",
	"claude-3-5-haiku-20241022":  "Claude 3.5 Haiku",
	"gpt-4o":                     "GPT-4o",
	"gpt-4o-mini":                "GPT-4o Mini",
	"o3-mini":                    "o3-mini",
	"o1":                         "o1",
	"deepseek-reasoner":          "DeepSeek-R1",
	"nemotron-3-super-free":      "Nemotron 3 Super Free",
	"big-pickle":                 "Big Pickle",
	"mimo-v2.5-free":             "MiMo V2.5 Free",
	"llama3.3":                   "Llama 3.3",
	"qwen2.5-coder":              "Qwen 2.5 Coder",
	"deepseek-r1":                "DeepSeek R1",
}

// Label returns the human label for a model identifier, falling back to the
// identifier itself when no curated label exists.
func Label(modelID string) string {
	if label, ok := curatedLabels[modelID]; ok {
		return label
	}
	return modelID
}

// LabeledModels returns a provider's fallback models paired with their labels,
// ready for a picker that shows names rather than raw identifiers.
func LabeledModels(provider string) []Preset {
	models := FallbackModelsForProvider(provider)
	labeled := make([]Preset, 0, len(models))
	for _, model := range models {
		labeled = append(labeled, Preset{Provider: strings.ToLower(strings.TrimSpace(provider)), Model: model, Name: Label(model)})
	}
	return labeled
}

// Preset is a curated model offered as a quick pick, carrying the human label
// shown for it.
type Preset struct {
	Provider string
	Model    string
	Name     string
}

// protonPrefixes mark the first-party proton catalog. These identifiers are
// metered even when they reuse a free-tier model's upstream base name, so they
// are checked before the free-name rules rather than after.
var protonPrefixes = []string{"proton/", "protonman/"}

// freeSuffixMarkers are the trailing conventions a catalog uses to advertise a
// model as free of charge.
var freeSuffixMarkers = []string{
	"/free",
	"-free",
}

// freeSubstringMarkers appear anywhere in an identifier, including mid-name and
// as a routing segment.
var freeSubstringMarkers = []string{
	":free",
	"(free)",
	"cline-free",
}

// freeExactNames are identifiers that are free despite carrying no marker.
var freeExactNames = map[string]bool{
	"free":       true,
	"big-pickle": true,
}

// freeRoutedNames are model basenames that are free only when served through a
// free upstream route. The same basename under a metered route is not free, so
// these are matched after stripping the routing prefix.
var freeRoutedNames = map[string]bool{
	"muse-spark-1.3-contributor": true,
	"deepseek-v4.1-flash":        true,
	"mimo-v2.6-flash":            true,
	"pixel-canary":               true,
	"space-bunny-alpha":          true,
	"kat-coder-pro":              true,
	"big-pickle":                 true,
}

// freeRoutePrefixes identify upstreams that serve free-tier models.
var freeRoutePrefixes = []string{
	"cline/",
	"cline-free/",
	"stealth/",
}

// IsFreeModel reports whether a model identifier denotes a free-tier model.
//
// The rules are evaluated in a deliberate order: the first-party proton catalog
// is never free, then explicit name markers win, then routed free-tier
// basenames. Checking proton first prevents a metered proton identifier from
// being classified as free purely because its basename matches a free upstream
// model.
func IsFreeModel(modelID string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	if lower == "" {
		return false
	}
	for _, prefix := range protonPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	for _, marker := range freeSubstringMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, marker := range freeSuffixMarkers {
		if strings.HasSuffix(lower, marker) {
			return true
		}
	}
	if freeExactNames[lower] {
		return true
	}
	for _, prefix := range freeRoutePrefixes {
		if strings.HasPrefix(lower, prefix) && freeRoutedNames[routedBase(lower)] {
			return true
		}
	}
	return false
}

// routedBase strips a routing prefix, returning the model basename.
func routedBase(modelID string) string {
	if idx := strings.LastIndex(modelID, "/"); idx != -1 {
		return modelID[idx+1:]
	}
	return modelID
}

// HasAnyFreeModel reports whether at least one identifier is free-tier.
func HasAnyFreeModel(models []string) bool {
	for _, model := range models {
		if IsFreeModel(model) {
			return true
		}
	}
	return false
}

// PartitionModels splits identifiers into free-tier and remaining models,
// preserving input order within each partition.
func PartitionModels(models []string) (freeModels, otherModels []string) {
	for _, model := range models {
		if IsFreeModel(model) {
			freeModels = append(freeModels, model)
		} else {
			otherModels = append(otherModels, model)
		}
	}
	return freeModels, otherModels
}
