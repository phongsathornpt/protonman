//go:build linux

package sandbox

import "testing"

func TestProbeCapabilitiesLinux(t *testing.T) {
	caps := ProbeCapabilities()
	if !caps.Native {
		t.Fatal("linux capability probe must report native support")
	}
	if caps.LandlockABI < 0 {
		t.Fatalf("Landlock ABI = %d, want >= 0", caps.LandlockABI)
	}
	if got := caps.SupportsLandlock(); got != (caps.LandlockABI > 0) {
		t.Fatalf("SupportsLandlock() = %v, ABI = %d", got, caps.LandlockABI)
	}
}
