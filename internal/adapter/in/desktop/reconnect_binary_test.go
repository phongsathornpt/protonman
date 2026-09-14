//go:build desktop

package desktop

import (
	"path/filepath"
	"testing"
)

func TestResolveACPBinaryForPrefersOverride(t *testing.T) {
	got := resolveACPBinaryFor(" /custom/protonman ", "/app/protonman-desktop", func(string) bool { return true })
	if got != "/custom/protonman" {
		t.Fatalf("binary = %q, want override", got)
	}
}

func TestResolveACPBinaryForUsesBundledRuntime(t *testing.T) {
	executable := filepath.Join(string(filepath.Separator), "opt", "protonman", "protonman-desktop")
	want := filepath.Join(filepath.Dir(executable), "libexec", "protonman")
	got := resolveACPBinaryFor("", executable, func(path string) bool { return path == want })
	if got != want {
		t.Fatalf("binary = %q, want bundled runtime %q", got, want)
	}
}

func TestResolveACPBinaryForFallsBackToPathLookup(t *testing.T) {
	got := resolveACPBinaryFor("", "/app/protonman-desktop", func(string) bool { return false })
	if got != "protonman" {
		t.Fatalf("binary = %q, want PATH fallback", got)
	}
}
