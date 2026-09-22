package paneutil

import (
	"testing"
)

func TestSmartFilterEmptyTermPreservesOrder(t *testing.T) {
	targets := []string{"one", "two", "three"}
	ranks := SmartFilter("", targets)
	if len(ranks) != 3 {
		t.Fatalf("ranks count = %d, want 3", len(ranks))
	}
	for i, r := range ranks {
		if r.Index != i {
			t.Fatalf("ranks[%d].Index = %d, want %d", i, r.Index, i)
		}
	}
}

func TestSmartFilterMultiWordOutOfOrder(t *testing.T) {
	targets := []string{
		"deepseek-v4-vision DeepSeek V4 Vision tools vision",
		"qwen-2.5-coder Qwen Coder",
		"claude-3-5-sonnet Claude Sonnet",
	}
	ranks := SmartFilter("vision deepseek", targets)
	if len(ranks) != 1 || ranks[0].Index != 0 {
		t.Fatalf("expected target 0 to match 'vision deepseek', got %#v", ranks)
	}
}

func TestSmartFilterPrefixPrioritizedOverSubstring(t *testing.T) {
	targets := []string{
		"custom-gpt-wrapper",
		"gpt-4o",
		"another-model",
	}
	ranks := SmartFilter("gpt", targets)
	if len(ranks) < 2 {
		t.Fatalf("expected at least 2 matches, got %#v", ranks)
	}
	if ranks[0].Index != 1 {
		t.Fatalf("expected 'gpt-4o' (index 1) ranked first, got %d", ranks[0].Index)
	}
}

func TestSmartFilterFuzzyFallback(t *testing.T) {
	targets := []string{
		"deepseek-chat",
		"qwen-flash",
	}
	ranks := SmartFilter("dpsek", targets)
	if len(ranks) != 1 || ranks[0].Index != 0 {
		t.Fatalf("expected fuzzy match on 'deepseek-chat', got %#v", ranks)
	}
}
