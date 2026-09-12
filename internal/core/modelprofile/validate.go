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
	if err := validateCompactionPolicy(profile.Compaction); err != nil {
		return err
	}
	effectiveCompaction := EffectiveCompactionPolicy(Resolved{
		ContextWindow: profile.ContextWindow, MaxInputTokens: profile.MaxInputTokens, Compaction: profile.Compaction,
	})
	if err := validateCompactionOrdering(effectiveCompaction); err != nil {
		return err
	}
	switch profile.Compatibility.ToolSchemaDialect {
	case ToolSchemaDefault, ToolSchemaGeminiSubset:
	default:
		return fmt.Errorf("invalid tool schema dialect %q", profile.Compatibility.ToolSchemaDialect)
	}
	return nil
}

func cloneProfile(profile Profile) Profile {
	profile.Match.ExactIDs = append([]string(nil), profile.Match.ExactIDs...)
	profile.Match.Prefixes = append([]string(nil), profile.Match.Prefixes...)
	profile.Reasoning.Levels = append(profile.Reasoning.Levels[:0:0], profile.Reasoning.Levels...)
	profile.AgentPolicy = AgentPolicy{}
	return profile
}

func validateCompactionPolicy(policy CompactionPolicy) error {
	for name, ratio := range map[string]float64{
		"soft":       policy.SoftThresholdRatio,
		"medium":     policy.MediumThresholdRatio,
		"aggressive": policy.AggressiveThresholdRatio,
		"emergency":  policy.EmergencyThresholdRatio,
		"target":     policy.TargetRatio,
	} {
		if ratio < 0 || ratio >= 1 {
			return fmt.Errorf("compaction %s ratio must be in [0,1)", name)
		}
	}
	if policy.MinRecentMessages < 0 {
		return fmt.Errorf("compaction minimum recent messages must be non-negative")
	}
	return nil
}

func validateCompactionOrdering(policy CompactionPolicy) error {
	if !(policy.TargetRatio < policy.SoftThresholdRatio &&
		policy.SoftThresholdRatio < policy.MediumThresholdRatio &&
		policy.MediumThresholdRatio < policy.AggressiveThresholdRatio &&
		policy.AggressiveThresholdRatio < policy.EmergencyThresholdRatio) {
		return fmt.Errorf("compaction ratios must satisfy target < soft < medium < aggressive < emergency")
	}
	return nil
}
