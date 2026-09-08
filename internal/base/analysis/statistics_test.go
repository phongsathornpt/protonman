package analysis

import (
	"math"
	"testing"
)

func TestRunningStats(t *testing.T) {
	var stats RunningStats
	for _, value := range []float64{1, 2, 3, 4} {
		stats.Add(value)
	}
	got := stats.Summary()
	if got.Count != 4 || got.Min != 1 || got.Max != 4 || got.Mean != 2.5 {
		t.Fatalf("summary = %+v", got)
	}
	if math.Abs(got.StdDev-1.2909944487) > 1e-9 {
		t.Fatalf("stddev = %v", got.StdDev)
	}
}
