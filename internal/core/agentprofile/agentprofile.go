// Package agentprofile owns the canonical agent-profile policy: profile
// identity, delegation rules, reasoning defaults, and tool capability scoping.
package agentprofile

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Profile classifies Protonman's primary and specialized engineering attributes.
type Profile string

const (
	// ProfileUniversal is the adaptive primary software engineering orchestrator.
	ProfileUniversal Profile = "universal"
	// ProfileStrength executes substantial implementation, fixes, and refactors.
	ProfileStrength Profile = "strength"
	// ProfileAgility performs fast, bounded, read-only exploration and tracing.
	ProfileAgility Profile = "agility"
	// ProfileIntelligence handles deep reasoning, difficult debugging, and high-risk engineering work.
	ProfileIntelligence Profile = "intelligence"
)

// Spec describes the domain policy attached to one profile.
type Spec struct {
	Profile           Profile
	Description       string
	Mutating          bool
	Reasoning         sdk.ReasoningEffort
	GroundingEvidence tool.EvidenceKind
	AllowedKinds      []tool.Kind
}

var specs = []Spec{
	{Profile: ProfileUniversal, Description: "adaptive primary software engineering orchestration", Mutating: true, Reasoning: sdk.ReasoningMedium, GroundingEvidence: tool.EvidenceNone},
	{Profile: ProfileStrength, Description: "substantial implementation, fixes, and focused refactors", Mutating: true, Reasoning: sdk.ReasoningMedium, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindGit, tool.KindWeb, tool.KindEdit, tool.KindBash}},
	{Profile: ProfileAgility, Description: "fast read-only exploration, tracing, and focused investigation", Reasoning: sdk.ReasoningMedium, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindGit, tool.KindWeb}},
	{Profile: ProfileIntelligence, Description: "deep reasoning, difficult debugging, architecture, and high-risk engineering", Mutating: true, Reasoning: sdk.ReasoningHigh, GroundingEvidence: tool.EvidenceWorkspace, AllowedKinds: []tool.Kind{tool.KindRead, tool.KindGrep, tool.KindGit, tool.KindWeb, tool.KindEdit, tool.KindBash}},
}

// SpecForProfile returns the domain policy for one profile.
func SpecForProfile(profile Profile) (Spec, bool) {
	for _, spec := range specs {
		if spec.Profile == profile {
			return spec, true
		}
	}
	return Spec{}, false
}

// SupportedProfiles returns every known profile, including the primary root.
func SupportedProfiles() []Profile {
	out := make([]Profile, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.Profile)
	}
	return out
}

// SubagentProfiles returns the delegatable profiles. Universal is the
// primary/root profile and cannot be spawned as a child.
func SubagentProfiles() []Profile {
	return []Profile{ProfileStrength, ProfileAgility, ProfileIntelligence}
}


// SubagentProfileNames returns the delegatable profile names.
func SubagentProfileNames() []string {
	profiles := SubagentProfiles()
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, string(profile))
	}
	return out
}

// SubagentProfileList joins the delegatable profile names.
func SubagentProfileList(separator string) string {
	return strings.Join(SubagentProfileNames(), separator)
}

// ProfileNames returns every known profile name.
func ProfileNames() []string {
	profiles := SupportedProfiles()
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, string(profile))
	}
	return out
}

// ProfileList joins every known profile name.
func ProfileList(separator string) string { return strings.Join(ProfileNames(), separator) }

// ProfileSchemaDescription renders the profile vocabulary for tool schemas.
func ProfileSchemaDescription() string {
	parts := make([]string, 0, len(specs))
	for _, spec := range specs {
		parts = append(parts, "'"+string(spec.Profile)+"' ("+spec.Description+")")
	}
	return "The Protonman agent profile: " + strings.Join(parts, ", ") + "."
}

// SubagentProfileSchemaDescription renders the delegatable vocabulary for schemas.
func SubagentProfileSchemaDescription() string {
	parts := make([]string, 0, len(SubagentProfiles()))
	for _, profile := range SubagentProfiles() {
		spec, _ := SpecForProfile(profile)
		parts = append(parts, "'"+string(profile)+"' ("+spec.Description+")")
	}
	return "The delegated subagent profile: " + strings.Join(parts, ", ") + "."
}

// Valid reports whether the profile is recognized.
func (p Profile) Valid() bool {
	_, ok := SpecForProfile(p)
	return ok
}

// IsMutating reports whether the profile may mutate workspace files or run shell commands.
func (p Profile) IsMutating() bool {
	spec, ok := SpecForProfile(p)
	return ok && spec.Mutating
}

// IsSubagent reports whether the profile may be delegated by Universal.
func (p Profile) IsSubagent() bool {
	switch p {
	case ProfileStrength, ProfileAgility, ProfileIntelligence:
		return true
	default:
		return false
	}
}

// ShortLabel returns the compact Dota-style attribute label used by the TUI.
func (p Profile) ShortLabel() string {
	switch p {
	case ProfileUniversal:
		return "UNI"
	case ProfileStrength:
		return "STR"
	case ProfileAgility:
		return "AGI"
	case ProfileIntelligence:
		return "INT"
	default:
		return "AGENT"
	}
}

// Allows reports whether the profile policy permits one tool kind.
func (s Spec) Allows(kind tool.Kind) bool {
	for _, allowed := range s.AllowedKinds {
		if kind == allowed {
			return true
		}
	}
	return false
}
