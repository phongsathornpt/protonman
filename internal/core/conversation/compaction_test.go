package conversation

import (
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"testing"
)

func testCompactionPolicy() modelprofile.CompactionPolicy {
	return modelprofile.CompactionPolicy{
		SoftThresholdRatio:       0.70,
		MediumThresholdRatio:     0.80,
		AggressiveThresholdRatio: 0.88,
		EmergencyThresholdRatio:  0.94,
		TargetRatio:              0.60,
		MinRecentMessages:        8,
	}
}

func TestPlanCompactionBelowThreshold(t *testing.T) {
	got := PlanCompaction(699, 1000, testCompactionPolicy())
	if got.Required() || got.Stage != CompactionNone {
		t.Fatalf("decision = %+v, want none", got)
	}
}

func TestPlanCompactionStagesAndTarget(t *testing.T) {
	tests := []struct {
		input int
		want  CompactionStage
	}{
		{700, CompactionSoft}, {800, CompactionMedium},
		{880, CompactionAggressive}, {940, CompactionEmergency},
	}
	for _, tt := range tests {
		got := PlanCompaction(tt.input, 1000, testCompactionPolicy())
		if got.Stage != tt.want || got.TargetTokens != 600 {
			t.Fatalf("input %d decision = %+v, want stage=%s target=600", tt.input, got, tt.want)
		}
	}
}
