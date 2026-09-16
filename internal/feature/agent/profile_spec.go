package agent

import agentprofile "github.com/phongsathornpt/protonman/internal/core/agentprofile"

// The canonical agent-profile vocabulary and policy live in
// internal/core/agentprofile. These aliases exist only so the coordinator,
// lifecycle, and registry code in this package can keep referring to the
// core-owned types through the package-local names. Do not re-export the
// core package's functions here; external consumers import core/agentprofile
// directly.

// ProfileSpec aliases the core-owned profile policy.
type ProfileSpec = agentprofile.Spec

// SpecForProfile returns the core-owned domain policy for one profile.
func SpecForProfile(profile Profile) (ProfileSpec, bool) {
	return agentprofile.SpecForProfile(profile)
}
