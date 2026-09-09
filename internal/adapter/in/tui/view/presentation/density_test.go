package presentation

import "testing"

func TestMinimalPolicySuppressesRoutineAndDebugDetail(t *testing.T) {
	policy := MinimalPolicy()
	if policy.ShowRoutineDetail() || policy.ShowDebugDetail() {
		t.Fatalf("minimal policy unexpectedly enables detail: %+v", policy)
	}
}

func TestDebugPolicyShowsAllDetail(t *testing.T) {
	policy := Policy{Density: DensityDebug}
	if !policy.ShowRoutineDetail() || !policy.ShowDebugDetail() {
		t.Fatalf("debug policy should enable all detail: %+v", policy)
	}
}
