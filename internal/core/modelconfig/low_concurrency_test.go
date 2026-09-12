package modelconfig

import "testing"

func TestParseLowConcurrencySetting(t *testing.T) {
	tests := []struct {
		input    string
		expected LowConcurrencySetting
		wantErr  bool
	}{
		{"", LowConcurrencyAuto, false},
		{"auto", LowConcurrencyAuto, false},
		{"AUTO", LowConcurrencyAuto, false},
		{"  auto  ", LowConcurrencyAuto, false},
		{"on", LowConcurrencyOn, false},
		{"enable", LowConcurrencyOn, false},
		{"enabled", LowConcurrencyOn, false},
		{"off", LowConcurrencyOff, false},
		{"disable", LowConcurrencyOff, false},
		{"disabled", LowConcurrencyOff, false},
		{"invalid", LowConcurrencyAuto, true},
		{"foo", LowConcurrencyAuto, true},
	}

	for _, tc := range tests {
		got, err := ParseLowConcurrencySetting(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseLowConcurrencySetting(%q) err = %v; wantErr = %v", tc.input, err, tc.wantErr)
		}
		if got != tc.expected {
			t.Errorf("ParseLowConcurrencySetting(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestLowConcurrencySettingString(t *testing.T) {
	if LowConcurrencyAuto.String() != "auto" {
		t.Errorf("Auto.String() = %q; want auto", LowConcurrencyAuto.String())
	}
	if LowConcurrencyOn.String() != "on" {
		t.Errorf("On.String() = %q; want on", LowConcurrencyOn.String())
	}
	if LowConcurrencyOff.String() != "off" {
		t.Errorf("Off.String() = %q; want off", LowConcurrencyOff.String())
	}
}

func TestLowConcurrencySettingEnabled(t *testing.T) {
	// On is always enabled
	if !LowConcurrencyOn.Enabled(false) || !LowConcurrencyOn.Enabled(true) {
		t.Error("LowConcurrencyOn should always be enabled")
	}
	// Off is always disabled
	if LowConcurrencyOff.Enabled(false) || LowConcurrencyOff.Enabled(true) {
		t.Error("LowConcurrencyOff should always be disabled")
	}
	// Auto delegates to autoRecommended
	if LowConcurrencyAuto.Enabled(false) {
		t.Error("LowConcurrencyAuto should be disabled when autoRecommended is false")
	}
	if !LowConcurrencyAuto.Enabled(true) {
		t.Error("LowConcurrencyAuto should be enabled when autoRecommended is true")
	}
}
