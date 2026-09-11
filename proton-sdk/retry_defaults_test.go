package protonsdk_test

import (
	"testing"
	"time"

	protonsdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// TestRetryZeroPolicyUsesCanonicalDefaults pins the SDK's zero-value fallbacks
// in DecideRetry/RetryDelay to its library defaults.
func TestRetryZeroPolicyUsesCanonicalDefaults(t *testing.T) {
	if protonsdk.DefaultRetryBaseBackoff != 5*time.Second {
		t.Fatalf("DefaultRetryBaseBackoff = %v", protonsdk.DefaultRetryBaseBackoff)
	}
	if protonsdk.DefaultRetryMaxBackoff != 60*time.Second {
		t.Fatalf("DefaultRetryMaxBackoff = %v", protonsdk.DefaultRetryMaxBackoff)
	}
	if protonsdk.DefaultRetryMaxAfter != 30*time.Second {
		t.Fatalf("DefaultRetryMaxAfter = %v", protonsdk.DefaultRetryMaxAfter)
	}
	if got := protonsdk.RetryDelay(1, protonsdk.RetryPolicy{}); got != protonsdk.DefaultRetryBaseBackoff {
		t.Fatalf("RetryDelay(1, zero) = %v, want %v", got, protonsdk.DefaultRetryBaseBackoff)
	}
	want := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second}
	for retry, expected := range want {
		if got := protonsdk.RetryDelay(retry+1, protonsdk.RetryPolicy{}); got != expected {
			t.Fatalf("RetryDelay(%d, zero) = %v, want %v", retry+1, got, expected)
		}
	}
}
