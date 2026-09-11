package model

import "testing"

func TestParseLowConcurrencySetting(t *testing.T) {
	tests := map[string]LowConcurrencySetting{
		"": LowConcurrencyAuto, "auto": LowConcurrencyAuto, "on": LowConcurrencyOn, "enabled": LowConcurrencyOn, "off": LowConcurrencyOff, "disabled": LowConcurrencyOff,
	}
	for raw, want := range tests {
		got, err := ParseLowConcurrencySetting(raw)
		if err != nil || got != want {
			t.Fatalf("ParseLowConcurrencySetting(%q) = %s, %v; want %s", raw, got, err, want)
		}
	}
	if _, err := ParseLowConcurrencySetting("turbo"); err == nil {
		t.Fatal("invalid low concurrency setting accepted")
	}
}

func TestLowConcurrencySettingEnabled(t *testing.T) {
	if !LowConcurrencyAuto.Enabled(true) {
		t.Fatal("auto should enable OpenCode free")
	}
	if LowConcurrencyAuto.Enabled(false) {
		t.Fatal("auto should not enable OpenCode paid")
	}
	if !LowConcurrencyOn.Enabled(false) {
		t.Fatal("on should force OpenCode paid")
	}
	if LowConcurrencyOff.Enabled(true) {
		t.Fatal("off should disable OpenCode free")
	}
	if !LowConcurrencyOn.Enabled(false) {
		t.Fatal("on should force any provider/model")
	}
}
