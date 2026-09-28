//go:build desktop || desktop_gio

package gioui

import (
	"testing"
	"time"
)

func TestResolveThemeMode(t *testing.T) {
	tests := []struct {
		configured string
		isDark     bool
		want       string
	}{
		{configured: "system", isDark: true, want: "dark"},
		{configured: "system", isDark: false, want: "light"},
		{configured: "auto", isDark: true, want: "dark"},
		{configured: "auto", isDark: false, want: "light"},
		{configured: "device", isDark: true, want: "dark"},
		{configured: "", isDark: true, want: "dark"},
		{configured: "", isDark: false, want: "light"},
		{configured: "dark", isDark: false, want: "dark"},
		{configured: "light", isDark: true, want: "light"},
		{configured: "slate-dark", isDark: false, want: "slate-dark"},
		{configured: "slate-light", isDark: true, want: "slate-light"},
	}

	for _, tt := range tests {
		got := resolveThemeMode(tt.configured, tt.isDark)
		if got != tt.want {
			t.Errorf("resolveThemeMode(%q, %v) = %q, want %q", tt.configured, tt.isDark, got, tt.want)
		}
	}
}

func TestNormalizedThemeMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: "system"},
		{input: "system", want: "system"},
		{input: "auto", want: "system"},
		{input: "device", want: "system"},
		{input: "dark", want: "dark"},
		{input: "DARK", want: "dark"},
		{input: "light", want: "light"},
		{input: "slate-dark", want: "slate-dark"},
		{input: "slate-light", want: "slate-light"},
		{input: "some-dark-variant", want: "dark"},
		{input: "unknown", want: "system"},
	}

	for _, tt := range tests {
		got := normalizedThemeMode(tt.input)
		if got != tt.want {
			t.Errorf("normalizedThemeMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsSystemDarkModeOverride(t *testing.T) {
	systemThemeMu.Lock()
	origDetector := overrideDetectorFn
	origTime := lastDetectionTime
	lastDetectionTime = time.Time{}
	overrideDetectorFn = func() bool { return false }
	systemThemeMu.Unlock()

	defer func() {
		systemThemeMu.Lock()
		overrideDetectorFn = origDetector
		lastDetectionTime = origTime
		systemThemeMu.Unlock()
	}()

	if isSystemDarkMode() {
		t.Fatal("expected isSystemDarkMode to return false when overridden")
	}

	systemThemeMu.Lock()
	overrideDetectorFn = func() bool { return true }
	systemThemeMu.Unlock()

	if !isSystemDarkMode() {
		t.Fatal("expected isSystemDarkMode to return true when overridden")
	}
}
