package conversation

import "github.com/phongsathornpt/protonman/internal/core/modelprofile"

type CompactionStage string

const (
	CompactionNone       CompactionStage = "none"
	CompactionSoft       CompactionStage = "soft"
	CompactionMedium     CompactionStage = "medium"
	CompactionAggressive CompactionStage = "aggressive"
	CompactionEmergency  CompactionStage = "emergency"
)

type CompactionDecision struct {
	Stage        CompactionStage
	InputTokens  int
	BudgetTokens int
	TargetTokens int
	UsageRatio   float64
}

func (d CompactionDecision) Required() bool { return d.Stage != CompactionNone }

// PlanCompaction classifies current usage and chooses a post-compaction target.
func PlanCompaction(inputTokens, budgetTokens int, policy modelprofile.CompactionPolicy) CompactionDecision {
	decision := CompactionDecision{Stage: CompactionNone, InputTokens: inputTokens, BudgetTokens: budgetTokens}
	if inputTokens <= 0 || budgetTokens <= 0 {
		return decision
	}
	decision.UsageRatio = float64(inputTokens) / float64(budgetTokens)
	switch {
	case decision.UsageRatio >= policy.EmergencyThresholdRatio:
		decision.Stage = CompactionEmergency
	case decision.UsageRatio >= policy.AggressiveThresholdRatio:
		decision.Stage = CompactionAggressive
	case decision.UsageRatio >= policy.MediumThresholdRatio:
		decision.Stage = CompactionMedium
	case decision.UsageRatio >= policy.SoftThresholdRatio:
		decision.Stage = CompactionSoft
	default:
		return decision
	}
	decision.TargetTokens = int(float64(budgetTokens) * policy.TargetRatio)
	if decision.TargetTokens < 1 {
		decision.TargetTokens = 1
	}
	return decision
}
