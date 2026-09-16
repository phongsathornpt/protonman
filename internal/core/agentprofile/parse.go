// Parse errors for agent-profile vocabulary.
package agentprofile

import (
	"fmt"
	"strings"
)

// ParseProfile converts a raw string into a validated Profile.
func ParseProfile(raw string) (Profile, error) {
	p := Profile(strings.TrimSpace(strings.ToLower(raw)))
	if !p.Valid() {
		return "", fmt.Errorf("unknown agent profile %q: supported profiles are %s", raw, ProfileList(", "))
	}
	return p, nil
}

// ParseSubagentProfile validates a delegated specialized attribute.
func ParseSubagentProfile(raw string) (Profile, error) {
	p, err := ParseProfile(raw)
	if err != nil {
		return "", err
	}
	if !p.IsSubagent() {
		return "", fmt.Errorf("profile %q cannot be delegated: supported subagents are %s", raw, SubagentProfileList(", "))
	}
	return p, nil
}
