package tool

import "testing"

func TestProviderSafeName(t *testing.T) {
	if got := ProviderSafeName("read_file"); got != "read_file" {
		t.Fatalf("safe name = %q", got)
	}
	first := ProviderSafeName("mcp.github.issue/search")
	second := ProviderSafeName("mcp.github.issue:search")
	if first == second || len(first) > 64 || len(second) > 64 {
		t.Fatalf("aliases collide or exceed limit: %q %q", first, second)
	}
	if first != ProviderSafeName("mcp.github.issue/search") {
		t.Fatal("alias is not deterministic")
	}
	long := ProviderSafeName("mcp." + string(make([]byte, 100)))
	if len(long) > 64 {
		t.Fatalf("long alias length = %d", len(long))
	}
}
