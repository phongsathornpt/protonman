package protonsdk_test

import (
	"testing"
	"time"

	protonsdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// TestRetryZeroPolicyUsesCanonicalDefaults pins the zero-value fallbacks in
// DecideRetry/RetryDelay to the SDK-owned canonical constants. The CLI runtime
// policy mirrors these values; change both sides together.
func TestRetryZeroPolicyUsesCanonicalDefaults(t *testing.T) {
	if protonsdk.DefaultRetryBaseBackoff != 500*time.Millisecond {
		t.Fatalf("DefaultRetryBaseBackoff = %v", protonsdk.DefaultRetryBaseBackoff)
	}
	if protonsdk.DefaultRetryMaxBackoff != 8*time.Second {
		t.Fatalf("DefaultRetryMaxBackoff = %v", protonsdk.DefaultRetryMaxBackoff)
	}
	if protonsdk.DefaultRetryMaxAfter != 30*time.Second {
		t.Fatalf("DefaultRetryMaxAfter = %v", protonsdk.DefaultRetryMaxAfter)
	}
	if got := protonsdk.RetryDelay(1, protonsdk.RetryPolicy{}); got != protonsdk.DefaultRetryBaseBackoff {
		t.Fatalf("RetryDelay(1, zero) = %v, want %v", got, protonsdk.DefaultRetryBaseBackoff)
	}
}
