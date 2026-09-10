package modelprofile

const defaultMinRecentMessages = 8

// EffectiveCompactionPolicy derives a context-size tier and overlays any
// explicit profile settings. This keeps model names out of compaction logic.
func EffectiveCompactionPolicy(resolved Resolved) CompactionPolicy {
	budget := resolved.MaxInputTokens
	if budget <= 0 {
		budget = resolved.ContextWindow
	}
	policy := tierCompactionPolicy(budget)
	overlayCompactionPolicy(&policy, resolved.Compaction)
	return policy
}

func tierCompactionPolicy(tokens int) CompactionPolicy {
	policy := CompactionPolicy{
		SoftThresholdRatio: 0.70, MediumThresholdRatio: 0.80,
		AggressiveThresholdRatio: 0.88, EmergencyThresholdRatio: 0.94,
		TargetRatio: 0.60, MinRecentMessages: defaultMinRecentMessages,
	}
	switch {
	case tokens <= 0:
		return policy
	case tokens <= 128_000:
		policy.SoftThresholdRatio, policy.MediumThresholdRatio = 0.68, 0.78
		policy.AggressiveThresholdRatio, policy.EmergencyThresholdRatio = 0.87, 0.94
		policy.TargetRatio = 0.56
	case tokens <= 256_000:
		policy.SoftThresholdRatio, policy.MediumThresholdRatio = 0.72, 0.81
		policy.AggressiveThresholdRatio, policy.EmergencyThresholdRatio = 0.89, 0.95
		policy.TargetRatio = 0.60
	case tokens <= 512_000:
		policy.SoftThresholdRatio, policy.MediumThresholdRatio = 0.76, 0.84
		policy.AggressiveThresholdRatio, policy.EmergencyThresholdRatio = 0.90, 0.95
		policy.TargetRatio = 0.62
	case tokens <= 1_000_000:
		policy.SoftThresholdRatio, policy.MediumThresholdRatio = 0.80, 0.87
		policy.AggressiveThresholdRatio, policy.EmergencyThresholdRatio = 0.92, 0.96
		policy.TargetRatio = 0.65
	default:
		policy.SoftThresholdRatio, policy.MediumThresholdRatio = 0.82, 0.88
		policy.AggressiveThresholdRatio, policy.EmergencyThresholdRatio = 0.93, 0.97
		policy.TargetRatio = 0.68
	}
	return policy
}

func overlayCompactionPolicy(dst *CompactionPolicy, src CompactionPolicy) {
	if src.SoftThresholdRatio > 0 {
		dst.SoftThresholdRatio = src.SoftThresholdRatio
	}
	if src.MediumThresholdRatio > 0 {
		dst.MediumThresholdRatio = src.MediumThresholdRatio
	}
	if src.AggressiveThresholdRatio > 0 {
		dst.AggressiveThresholdRatio = src.AggressiveThresholdRatio
	}
	if src.EmergencyThresholdRatio > 0 {
		dst.EmergencyThresholdRatio = src.EmergencyThresholdRatio
	}
	if src.TargetRatio > 0 {
		dst.TargetRatio = src.TargetRatio
	}
	if src.MinRecentMessages > 0 {
		dst.MinRecentMessages = src.MinRecentMessages
	}
}
