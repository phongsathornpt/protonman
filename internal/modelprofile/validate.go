package modelprofile

import (
	"fmt"
	"strings"
)

func validateProfile(profile Profile) error {
	if strings.TrimSpace(profile.Match.Provider) == "" && len(profile.Match.ExactIDs) == 0 && len(profile.Match.Prefixes) == 0 {
		return fmt.Errorf("matcher is required")
	}
	for _, effort := range profile.Reasoning.Levels {
		if !effort.Valid() || effort == "" {
			return fmt.Errorf("invalid reasoning level %q", effort)
		}
	}
	if profile.Reasoning.Default != "" && !profile.Reasoning.Default.Valid() {
		return fmt.Errorf("invalid default reasoning effort %q", profile.Reasoning.Default)
	}
	return nil
}

func cloneProfile(profile Profile) Profile {
	profile.Match.ExactIDs = append([]string(nil), profile.Match.ExactIDs...)
	profile.Match.Prefixes = append([]string(nil), profile.Match.Prefixes...)
	profile.Reasoning.Levels = append(profile.Reasoning.Levels[:0:0], profile.Reasoning.Levels...)
	profile.PromptHints = append([]string(nil), profile.PromptHints...)
	return profile
}
