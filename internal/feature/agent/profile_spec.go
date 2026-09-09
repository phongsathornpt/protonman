package agent

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type ProfileSpec struct {
	Profile           Profile
	Description       string
	Mutating          bool
	Reasoning         sdk.ReasoningEffort
	GroundingEvidence tool.EvidenceKind
	AllowedKinds      []tool.Kind
}

var profileSpecs = []ProfileSpec{
	{Profile: ProfileUniversal, Description: "adaptive primary software engineering orchestration", Mutating: true, Reasoning: sdk.ReasoningMedium, GroundingEvidence: tool.EvidenceNone},
	{Profile: ProfileStrength, Description: "substantial implementation, fixes, and focused refactors", Mutating: true, Reasoning: sdk.ReasoningMedium, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWeb, tool.KindEdit, tool.KindBash}},
	{Profile: ProfileAgility, Description: "fast read-only exploration, tracing, and focused investigation", Reasoning: sdk.ReasoningMedium, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWeb}},
	{Profile: ProfileIntelligence, Description: "deep reasoning, difficult debugging, architecture, and high-risk engineering", Mutating: true, Reasoning: sdk.ReasoningHigh, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWeb, tool.KindEdit, tool.KindBash}},
}

func SpecForProfile(profile Profile) (ProfileSpec, bool) {
	for _, spec := range profileSpecs {
		if spec.Profile == profile {
			return spec, true
		}
	}
	return ProfileSpec{}, false
}

func SupportedProfiles() []Profile {
	out := make([]Profile, 0, len(profileSpecs))
	for _, spec := range profileSpecs {
		out = append(out, spec.Profile)
	}
	return out
}

func SubagentProfiles() []Profile {
	return []Profile{ProfileStrength, ProfileAgility, ProfileIntelligence}
}

func SubagentProfileNames() []string {
	profiles := SubagentProfiles()
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, string(profile))
	}
	return out
}

func SubagentProfileList(separator string) string {
	return strings.Join(SubagentProfileNames(), separator)
}

func ProfileNames() []string {
	profiles := SupportedProfiles()
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, string(profile))
	}
	return out
}

func ProfileList(separator string) string { return strings.Join(ProfileNames(), separator) }

func ProfileSchemaDescription() string {
	parts := make([]string, 0, len(profileSpecs))
	for _, spec := range profileSpecs {
		parts = append(parts, "'"+string(spec.Profile)+"' ("+spec.Description+")")
	}
	return "The Protonman agent profile: " + strings.Join(parts, ", ") + "."
}

func SubagentProfileSchemaDescription() string {
	parts := make([]string, 0, len(SubagentProfiles()))
	for _, profile := range SubagentProfiles() {
		spec, _ := SpecForProfile(profile)
		parts = append(parts, "'"+string(profile)+"' ("+spec.Description+")")
	}
	return "The delegated subagent profile: " + strings.Join(parts, ", ") + "."
}

func (s ProfileSpec) Allows(kind tool.Kind) bool {
	for _, allowed := range s.AllowedKinds {
		if kind == allowed {
			return true
		}
	}
	return false
}
