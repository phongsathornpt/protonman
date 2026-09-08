package buildinfo

import (
	"strings"
	"testing"
)

func TestUserAgentsShareResolvedVersion(t *testing.T) {
	ua := UserAgent()
	if !strings.HasPrefix(ua, Name+"/") {
		t.Fatalf("UserAgent() = %q", ua)
	}
	if got := WebUserAgent(); !strings.HasPrefix(got, ua+" (+") || !strings.Contains(got, Repository) {
		t.Fatalf("WebUserAgent() = %q", got)
	}
}

func TestVersionPrefersInjectedValue(t *testing.T) {
	previous := version
	version = "v1.2.3"
	t.Cleanup(func() { version = previous })

	if got := Version(); got != "1.2.3" {
		t.Fatalf("Version() = %q, want 1.2.3", got)
	}
}

func TestVersionPreservesGitDescribeSuffix(t *testing.T) {
	previous := version
	version = "v1.2.3-4-gabcdef-dirty"
	t.Cleanup(func() { version = previous })

	if got := Version(); got != "1.2.3-4-gabcdef-dirty" {
		t.Fatalf("Version() = %q, want git describe version", got)
	}
}
