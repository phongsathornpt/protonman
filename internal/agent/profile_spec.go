package agent

import (
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
)

type ProfileSpec struct {
	Profile      Profile
	Description  string
	Mutating     bool
	AllowedKinds []tool.Kind
}

var profileSpecs = []ProfileSpec{
	{Profile: ProfileExplorer, Description: "read-only search and inspection", AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch}},
	{Profile: ProfileReviewer, Description: "read-only code and security review", AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep}},
	{Profile: ProfileWorker, Description: "code modifications and commands", Mutating: true, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch, tool.KindEdit, tool.KindBash}},
	{Profile: ProfilePOW, Description: "high-velocity pragmatic execution", Mutating: true, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch, tool.KindEdit, tool.KindBash}},
	{Profile: ProfileDEX, Description: "defensive zero-regression engineering", Mutating: true, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch, tool.KindEdit, tool.KindBash}},
	{Profile: ProfileINT, Description: "deep architectural reasoning", AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindWebFetch, tool.KindWebSearch}},
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
