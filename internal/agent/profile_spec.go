package agent

import (
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
	sdk "github.com/projectTHORN/proton/proton-sdk"
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
	{Profile: ProfilePOW, Description: "implementation, fixes, and focused refactors", Mutating: true, Reasoning: sdk.ReasoningMedium, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch, tool.KindEdit, tool.KindBash}},
	{Profile: ProfileINT, Description: "read-only investigation, tracing, research, and review", Reasoning: sdk.ReasoningHigh, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch}},
	{Profile: ProfileDEX, Description: "complex design, difficult debugging, and high-risk engineering", Mutating: true, Reasoning: sdk.ReasoningHigh, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch, tool.KindEdit, tool.KindBash}},
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
	return "The subagent profile: " + strings.Join(parts, ", ") + "."
}

func (s ProfileSpec) Allows(kind tool.Kind) bool {
	for _, allowed := range s.AllowedKinds {
		if kind == allowed {
			return true
		}
	}
	return false
}
