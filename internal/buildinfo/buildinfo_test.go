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
